package main

import (
	"context"
	"fiber/config"
	"fiber/database"
	"fiber/structs"
	"fiber/worker"
	"fmt"
	"log"
	"os"
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
var CAROUSEL_SELECTORS = structs.CarouselPhotos{}
var REF_SELECTOR string
var NAME string

var dbPool *pgxpool.Pool

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

	if len(req.Selectors.Photos) > 0 {
		PHOTO_SELECTORS = req.Selectors.Photos
	}
	if len(req.Selectors.Content) > 0 {
		DESCRIPTION_SELECTORS = req.Selectors.Content
	}

	if len(req.Selectors.CarouselPhotos.Full) > 0 || len(req.Selectors.CarouselPhotos.Thumbnail) > 0 {
		CAROUSEL_SELECTORS = req.Selectors.CarouselPhotos
	}

	NAME = req.Name
	REF_SELECTOR = req.Selectors.Ref

	go func(r structs.ScrapeRequest) {
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

		start := time.Now()
		numWorkers := 10
		log.Printf("Background job for %s", NAME)

		urlChan := make(chan string, len(r.URLs))
		resultChan := make(chan structs.ScrapedData, len(r.URLs))
		var wg sync.WaitGroup

		log.Printf("Workers working...")

		for i := range numWorkers {
			wg.Add(1)
			go worker.Worker(structs.WorkerInput{
				Name:       NAME,
				Wg:         &wg,
				WorkerId:   i,
				Browser:    browser,
				UrlChan:    urlChan,
				ResultChan: resultChan,
				Selectors: structs.Selectors{
					Content:        DESCRIPTION_SELECTORS,
					Photos:         PHOTO_SELECTORS,
					CarouselPhotos: CAROUSEL_SELECTORS,
					Ref:            REF_SELECTOR,
				},
			})
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
				go worker.Worker(structs.WorkerInput{
					Name:       fmt.Sprintf("Retry %s", NAME),
					Wg:         &retryWg,
					WorkerId:   i,
					Browser:    browser,
					UrlChan:    retryUrlChan,
					ResultChan: retryResultChan,
					Selectors: structs.Selectors{
						Content:        DESCRIPTION_SELECTORS,
						Photos:         PHOTO_SELECTORS,
						CarouselPhotos: CAROUSEL_SELECTORS,
						Ref:            REF_SELECTOR,
					},
				})
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

			if len(retryFailedLinks) > 0 {
				ctx := context.Background()
				err := database.UpdateScrappedInfo(dbPool, ctx, structs.ScrappedInfo{LinksFailed: retryFailedLinks})
				if err != nil {
					log.Printf("Failed to update scrapped info: %v", err)
				}
			}
		}

		elapsed := time.Since(start)
		log.Printf("Background job finished: processed %d results, saved %d, errors %d, elapsed time %s", len(results), savedCount, errorCount, elapsed.Round(elapsed))
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

	app.Post("/scrape", scrapeHandler)

	serverError := app.Listen(getPort())
	if serverError != nil {
		log.Fatalf("Failed to start server: %v", serverError)
	}
}
