package structs

import (
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

type ScrappedInfo struct {
	ID            int       `json:"id"`
	TotalListings int       `json:"total_listings"`
	TotalPages    int       `json:"total_pages"`
	Agency        string    `json:"agency"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LinksFailed   []string  `json:"links_failed"`
}

type Listing struct {
	ID               int       `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Name             string    `json:"name"`
	Link             string    `json:"link"`
	Image            string    `json:"image"`
	Address          string    `json:"address"`
	Price            float64   `json:"price"`
	Area             float64   `json:"area"`
	Bedrooms         int       `json:"bedrooms"`
	Type             string    `json:"type"`
	ForSale          bool      `json:"for_sale"`
	Parking          int       `json:"parking"`
	Content          string    `json:"content"`
	Photos           []string  `json:"photos"`
	Agency           string    `json:"agency"`
	Bathrooms        int       `json:"bathrooms"`
	Ref              string    `json:"ref"`
	PlaceholderImage string    `json:"placeholder_image"`
	AgentId          int       `json:"agent_id"`
	Published        bool      `json:"published"`
}

type ScrapedData struct {
	URL     string   `json:"url"`
	Ref     string   `json:"ref"`
	Content string   `json:"content"`
	Photos  []string `json:"photos"`
	Error   string   `json:"error,omitempty"`
}

type CarouselPhotos struct {
	Thumbnail []string `json:"thumbnail,omitempty"`
	Full      []string `json:"full,omitempty"`
}

type Selectors struct {
	Ref            string         `json:"ref,omitempty"`
	Photos         []string       `json:"photos,omitempty"`
	Content        []string       `json:"content,omitempty"`
	CarouselPhotos CarouselPhotos `json:"carousel_photos"`
}

type ScrapeRequest struct {
	Name      string    `json:"name"`
	URLs      []string  `json:"urls"`
	Selectors Selectors `json:"selectors"`
}

type ScrapeResponse struct {
	Results []ScrapedData `json:"results"`
	Total   int           `json:"total"`
}

type WorkerInput struct {
	Name       string
	WorkerId   int
	Browser    playwright.Browser
	UrlChan    <-chan string
	ResultChan chan<- ScrapedData
	Wg         *sync.WaitGroup
	Selectors  Selectors
}
