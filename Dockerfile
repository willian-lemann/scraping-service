FROM golang:1.24-bullseye

WORKDIR /app

# Install Playwright dependencies
RUN apt-get update && apt-get install -y \
   wget \
   ca-certificates \
   fonts-liberation \
   libasound2 \
   libatk-bridge2.0-0 \
   libatk1.0-0 \
   libatspi2.0-0 \
   libcups2 \
   libdbus-1-3 \
   libdrm2 \
   libgbm1 \
   libgtk-3-0 \
   libnspr4 \
   libnss3 \
   libwayland-client0 \
   libxcomposite1 \
   libxdamage1 \
   libxfixes3 \
   libxkbcommon0 \
   libxrandr2 \
   xdg-utils \
   libu2f-udev \
   libvulkan1 \
   && rm -rf /var/lib/apt/lists/*

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o scraping-service .

# Install Playwright browsers
RUN go run github.com/playwright-community/playwright-go/cmd/playwright@latest install --with-deps chromium

# Expose port
EXPOSE 8080

# Run the application
CMD ["./scraping-service"]