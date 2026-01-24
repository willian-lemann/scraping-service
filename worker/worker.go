package worker

import (
	"fiber/structs"

	"log"
	"regexp"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

func extractFromURLRef(url string, regexSelector string) string {
	re := regexp.MustCompile(regexSelector)
	matches := re.FindStringSubmatch(url)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func scrapeURL(page playwright.Page, url string, selectors structs.Selectors) structs.ScrapedData {
	result := structs.ScrapedData{
		URL:     url,
		Ref:     extractFromURLRef(url, selectors.Ref),
		Photos:  []string{},
		Content: "",
	}

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(10000),
	}); err != nil {
		result.Error = "Failed to navigate: " + err.Error()
		return result
	}

	for _, selector := range selectors.Content {
		locator := page.Locator(selector).First()
		text, err := locator.InnerText(playwright.LocatorInnerTextOptions{
			Timeout: playwright.Float(3000),
		})

		if err == nil && strings.TrimSpace(text) != "" {
			result.Content = strings.TrimSpace(text)
			break
		}
	}

	for _, selector := range selectors.Photos {
		imgs := page.Locator(selector)
		count, err := imgs.Count()

		if err != nil {
			continue
		}

		for i := range count {
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

func scrapeURLBonavista(page playwright.Page, url string) structs.ScrapedData {
	result := structs.ScrapedData{
		URL:     url,
		Ref:     extractFromURLRef(url, `/([^/]+)$`),
		Photos:  []string{},
		Content: "",
	}

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateLoad,
		Timeout:   playwright.Float(6000),
	}); err != nil {
		log.Printf("Failed to navigate to Bonavista URL %s: %v", url, err)
		return result
	}

	// Get content from the property description
	contentLocator := page.Locator("p.property-description_text__xdJhn").First()
	text, err := contentLocator.InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(3000),
	})

	if err == nil && strings.TrimSpace(text) != "" {
		result.Content = strings.TrimSpace(text)
	}

	page.Locator("[class*='media-gallery-buttons_active__']").WaitFor()

	// Click the photos button to load the gallery
	photosButton := page.Locator("[class*='multi-multimedia-gallery_mainGallery__XIX0x']").First()

	if err := photosButton.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		log.Printf("Failed to click photos button for Bonavista URL %s: %v", url, err)
		return result
	}

	time.Sleep(500 * time.Millisecond)

	imgs := page.Locator("div.media-gallery-tour_galleryMosaic__n9mIr img")
	count, err := imgs.Count()

	if err != nil {
		log.Printf("Failed to count images for Bonavista URL %s: %v", url, err)
		return result
	}

	for i := range count {
		img := imgs.Nth(i)
		src, err := img.GetAttribute("src")

		if err == nil && src != "" && strings.Contains(src, "http") {
			result.Photos = append(result.Photos, src)
		}
	}

	return result
}

func Worker(input structs.WorkerInput) {
	defer input.Wg.Done()

	context, err := input.Browser.NewContext()
	if err != nil {
		log.Printf("%s %d: Failed to create context: %v", input.Name, input.WorkerId, err)
		return
	}
	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		log.Printf("%s %d: Failed to create page: %v", input.Name, input.WorkerId, err)
		return
	}

	for url := range input.UrlChan {
		var result structs.ScrapedData
		if strings.Contains(url, "bonavista") {
			result = scrapeURLBonavista(page, url)
		} else {
			result = scrapeURL(page, url, input.Selectors)
		}
		input.ResultChan <- result
		time.Sleep(100 * time.Millisecond)
	}
}
