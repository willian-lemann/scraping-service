package main

import (
	"context"
	"fiber/config"
	"fiber/database"
	"fiber/structs"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/playwright-community/playwright-go"
)

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":8080"
	} else {
		port = ":" + port
	}

	return port
}

var DESCRIPTION_SELECTORS = []string{}

var PHOTO_SELECTORS = []string{}

var dbPool *pgxpool.Pool

func extractFromURLRef(url string) string {
	re := regexp.MustCompile(`/imovel/venda/(\d+)/`)
	matches := re.FindStringSubmatch(url)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func scrapeURL(page playwright.Page, url string) structs.ScrapedData {
	result := structs.ScrapedData{
		URL:     url,
		Ref:     extractFromURLRef(url),
		Photos:  []string{},
		Content: "",
	}

	// Navigate to URL
	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(10000),
	}); err != nil {
		result.Error = "Failed to navigate: " + err.Error()
		return result
	}

	for _, selector := range DESCRIPTION_SELECTORS {
		locator := page.Locator(selector).First()
		text, err := locator.InnerText(playwright.LocatorInnerTextOptions{
			Timeout: playwright.Float(3000),
		})
		if err == nil && strings.TrimSpace(text) != "" {
			result.Content = strings.TrimSpace(text)
			break
		}
	}

	for _, selector := range PHOTO_SELECTORS {
		imgs := page.Locator(selector)
		count, err := imgs.Count()
		if err != nil {
			continue
		}

		for i := 0; i < count; i++ {
			img := imgs.Nth(i)
			src, err := img.GetAttribute("src")
			if err == nil && src != "" && strings.Contains(src, "http") {
				result.Photos = append(result.Photos, src)
			}
		}

		if len(result.Photos) > 0 {
			break
		}
	}

	return result
}

// single-job coordination: block all requests while a job runs
var jobLock sync.RWMutex
var jobFlagMutex sync.Mutex
var jobRunning bool

func scrapeHandler(c *fiber.Ctx) error {
	var req structs.ScrapeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if len(req.URLs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No URLs provided",
		})
	}

	if len(req.Selectors.Content) == 0 && len(req.Selectors.Photos) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No URLs provided",
		})
	}

	// ensure only one job can start
	jobFlagMutex.Lock()
	if jobRunning {
		jobFlagMutex.Unlock()
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error": "Another job is running",
		})
	}
	jobRunning = true
	jobFlagMutex.Unlock()

	// set selectors up-front (safe because only one job at a time)
	if len(req.Selectors.Photos) > 0 {
		PHOTO_SELECTORS = req.Selectors.Photos
	}
	if len(req.Selectors.Content) > 0 {
		DESCRIPTION_SELECTORS = req.Selectors.Content
	}

	// run the full scraping job in background and immediately return
	go func(r structs.ScrapeRequest) {
		// acquire write lock so all other requests block until the job finishes
		jobLock.Lock()
		defer func() {
			jobLock.Unlock()
			jobFlagMutex.Lock()
			jobRunning = false
			jobFlagMutex.Unlock()
		}()

		pw, err := playwright.Run()
		if err != nil {
			log.Printf("Background job: Failed to start playwright: %v", err)
			return
		}
		defer pw.Stop()

		browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(true),
		})
		if err != nil {
			log.Printf("Background job: Failed to launch browser: %v", err)
			return
		}
		defer browser.Close()

		numWorkers := 10
		log.Printf("Background job: starting %d workers", numWorkers)

		urlChan := make(chan string, len(r.URLs))
		resultChan := make(chan structs.ScrapedData, len(r.URLs))
		var wg sync.WaitGroup

		log.Printf("Workers working...")

		for i := range numWorkers {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				context, err := browser.NewContext()
				if err != nil {
					log.Printf("Worker %d: Failed to create context: %v", workerID, err)
					return
				}
				defer context.Close()

				page, err := context.NewPage()
				if err != nil {
					log.Printf("Worker %d: Failed to create page: %v", workerID, err)
					return
				}

				for url := range urlChan {
					result := scrapeURL(page, url)
					resultChan <- result
					time.Sleep(100 * time.Millisecond)
				}
			}(i)
		}

		go func() {
			for _, url := range r.URLs {
				urlChan <- url
			}
			close(urlChan)
		}()

		go func() {
			wg.Wait()
			close(resultChan)
		}()

		results := make([]structs.ScrapedData, 0, len(r.URLs))
		savedCount := 0
		failedLinks := []string{}
		failedToSaveLinks := []string{}
		errorCount := 0

		for result := range resultChan {
			results = append(results, result)

			if result.Error == "" {
				ctx := context.Background()

				if err := database.UpdateListing(dbPool, ctx, result.Ref, structs.Listing{
					Content: result.Content,
					Photos:  result.Photos,
				}); err != nil {
					log.Printf("Failed to save listing %s: %v", result.URL, err)
					errorCount++
					failedToSaveLinks = append(failedToSaveLinks, result.URL)
				} else {
					savedCount++
				}
			} else {
				errorCount++
				failedLinks = append(failedLinks, result.URL)
			}
		}

		if len(failedLinks) > 0 {
			log.Printf("Found %d failed links", len(failedLinks))

			retryUrlChan := make(chan string, len(failedLinks))
			retryResultChan := make(chan structs.ScrapedData, len(failedLinks))
			var retryWg sync.WaitGroup

			numRetryWorkers := 5

			log.Printf("Retry workers working")

			for i := range numRetryWorkers {
				retryWg.Add(1)
				go func(workerID int) {
					defer retryWg.Done()

					context, err := browser.NewContext()
					if err != nil {
						log.Printf("Retry Worker %d: Failed to create context: %v", workerID, err)
						return
					}
					defer context.Close()

					page, err := context.NewPage()
					if err != nil {
						log.Printf("Retry Worker %d: Failed to create page: %v", workerID, err)
						return
					}

					for url := range retryUrlChan {
						result := scrapeURL(page, url)
						retryResultChan <- result
						time.Sleep(200 * time.Millisecond)
					}
				}(i)
			}

			// Send retry URLs
			go func() {
				for _, url := range failedLinks {
					retryUrlChan <- url
				}
				close(retryUrlChan)
			}()

			// Wait for retry workers to finish
			go func() {
				retryWg.Wait()
				close(retryResultChan)
			}()

			retryFailedLinks := []string{}
			for result := range retryResultChan {
				if result.Error == "" {
					ctx := context.Background()
					if err := database.UpdateListing(dbPool, ctx, result.Ref, structs.Listing{
						Content: result.Content,
						Photos:  result.Photos,
					}); err != nil {
						log.Printf("Failed to save retried listing %s: %v", result.URL, err)
						retryFailedLinks = append(retryFailedLinks, result.URL)
					} else {
						savedCount++
						errorCount-- // decrease error count for successful retry
					}
				} else {
					log.Printf("Retry failed for %s: %v", result.URL, result.Error)
					retryFailedLinks = append(retryFailedLinks, result.URL)
				}
			}

			// Update database with final failed links after retry
			if len(retryFailedLinks) > 0 {
				ctx := context.Background()
				err := database.UpdateScrappedInfo(dbPool, ctx, structs.ScrappedInfo{LinksFailed: retryFailedLinks})
				if err != nil {
					log.Printf("Failed to update scrapped info: %v", err)
				}
			}
		}

		log.Printf("Background job finished: processed %d results, saved %d, errors %d", len(results), savedCount, errorCount)
	}(req)

	// respond immediately to free the HTTP connection
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"status": "accepted",
	})
}

func main() {
	config.LoadEnvs()

	// Initialize database
	var err error
	dbPool, err = database.InitDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	defer dbPool.Close()

	app := fiber.New()

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"ok": "true",
		})
	})

	// middleware to block requests while a job holds the write lock
	app.Use(func(c *fiber.Ctx) error {
		jobLock.RLock()
		defer jobLock.RUnlock()
		return c.Next()
	})

	app.Post("/scrape", scrapeHandler)

	serverError := app.Listen(getPort())
	if serverError != nil {
		log.Fatalf("Failed to start server: %v", serverError)
	}
}
