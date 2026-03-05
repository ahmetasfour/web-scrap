# Web Scraper Project - Instructions

## Project Overview

This is a web scraping application designed to extract contact information (emails and phone numbers) for German property management companies from public directories like GelbeSeiten and DasOertliche.

### Architecture

**Backend (Go):**
- **Framework:** Fiber v2 (high-performance web framework)
- **Scraping Engine:** Colly v2 (powerful web scraping framework)
- **Logging:** Zap (structured logging)
- **Configuration:** JSON-based config file
- **Features:**
  - Concurrent scraping with rate limiting
  - Fuzzy name matching for company identification
  - Retry logic with exponential backoff
  - Server-Sent Events (SSE) for real-time streaming results
  - History persistence (last 500 results)

**Frontend (Next.js):**
- **Framework:** Next.js 15 with App Router
- **UI Library:** Ant Design v5
- **Styling:** Tailwind CSS
- **Language:** TypeScript
- **State Management:** React hooks + SWR for data fetching
- **File Handling:** XLSX for Excel file processing

### Data Flow

1. User uploads Excel file containing company data (German property management companies)
2. Frontend parses Excel and displays company table
3. User selects companies to scrape
4. Frontend sends selected companies to backend via SSE endpoint
5. Backend scrapes multiple sources concurrently:
   - GelbeSeiten.de (German Yellow Pages)
   - DasOertliche.de (German local directory)
6. Results are streamed back in real-time via SSE
7. Frontend displays results with status updates

### Key Components

#### Backend Components

**Scrapers:**
- `GelbeSeitenScraper`: Scrapes gelbeseiten.de
- `DasOertlicheScraper`: Scrapes dasoertliche.de
- Each scraper implements the `Source` interface

**Engine:**
- Manages concurrent scraping with configurable concurrency
- Handles rate limiting and delays
- Implements retry logic
- Uses shared HTTP transport for connection pooling

**Matcher:**
- Normalizes company names (removes legal forms like GmbH, AG)
- Calculates Levenshtein distance for fuzzy matching
- Configurable similarity threshold (default: 0.55)

**Models:**
- `Company`: Represents input company data from Excel
- `ScrapeResult`: Contains original company data + scraped contact info
- `ContactInfo`: Holds emails and phones from a single source

#### Frontend Components

**Main Components:**
- `FileUpload`: Handles Excel file upload and parsing
- `CompanyTable`: Displays companies with selection checkboxes
- `ScrapeResults`: Shows scraping progress and results

**Data Layer:**
- `scraper.ts`: Handles API communication with SSE streaming
- `index.ts`: Type definitions
- `useRequest.ts`: SWR-based data fetching hooks

### Configuration

**Backend Config (`api/config.json`):**
```json
{
  "server": {
    "domain": "",
    "port": 8080
  },
  "scraper": {
    "concurrency": 5,
    "requestDelayMs": 2000,
    "randomDelayMs": 1000,
    "retryCount": 2,
    "requestTimeoutSec": 30
  },
  "matcher": {
    "threshold": 0.55
  }
}
```

**Environment Variables:**
- `PORT`: Override server port (default: 8080)

### API Endpoints

- `POST /api/scrape`: Synchronous scraping (returns all results at once)
- `POST /api/scrape/stream`: Asynchronous streaming via SSE
- `GET /api/scrape/history`: Get last 500 scraping results
- `GET /health`: Health check endpoint

### Development Setup

#### Prerequisites
- Go 1.24+
- Node.js 18+
- npm or yarn

#### Backend Setup
```bash
cd api
go mod download
go run main.go
```

#### Frontend Setup
```bash
cd frontend
npm install
npm run dev
```

#### Running Both Services
```bash
# Terminal 1 - Backend
cd api && go run main.go

# Terminal 2 - Frontend
cd frontend && npm run dev
```

Frontend will be available at `http://localhost:3000`
Backend API at `http://localhost:8080`

### Excel File Format

The application expects Excel files with the following columns (German headers):
- `ID`
- `EnObjekt`
- `ReName` (Company Name)
- `ReName2` (Alternative Name)
- `ObjektRechnung`
- `ReOrt` (City)
- `ReHausnummer` (House Number)
- `RePlz` (Postal Code)
- `ReStrasse` (Street)
- `ReNummer`
- `Email`
- `Telefonnummer` (Phone)

### Scraping Logic

1. For each company, construct search URLs for each scraper
2. Search for company name + city
3. Extract candidate companies from search results
4. Use fuzzy matching to find the best match
5. Extract contact information from matched company page
6. Return emails and phones with source attribution

### Error Handling

- Network timeouts and retries
- 404 errors treated as "not found" (not retriable)
- Rate limiting with configurable delays
- Concurrent scraping with worker pool
- Graceful degradation for partial failures

### Performance Considerations

- Concurrent scraping (configurable concurrency)
- Shared HTTP transport for connection reuse
- Rate limiting to avoid being blocked
- Random delays to mimic human behavior
- Streaming results to avoid memory issues with large datasets

### Security & Ethics

- Respect robots.txt
- Use reasonable delays between requests
- Only scrape public directories
- Data is for legitimate business purposes (property management contact info)

### Future Enhancements

- Add more German directories (Yelp Germany, etc.)
- Implement caching layer for repeated searches
- Add export functionality (CSV/Excel output)
- User authentication and API keys
- Rate limiting per IP
- Monitoring and metrics
- Docker containerization

### Dependencies

**Go Backend:**
- `github.com/gofiber/fiber/v2`: Web framework
- `github.com/gocolly/colly/v2`: Web scraping
- `github.com/ahmet4dev/gol-lib`: Custom utilities
- `go.uber.org/zap`: Logging

**Frontend:**
- `next`: React framework
- `antd`: UI components
- `tailwindcss`: Styling
- `swr`: Data fetching
- `xlsx`: Excel processing
- `react`: UI library

### Code Quality

- Go: Structured logging, proper error handling, concurrent patterns
- TypeScript: Strict typing, modern React patterns
- Clean architecture with separation of concerns
- Comprehensive error handling and user feedback

### Testing

Currently minimal testing. Recommended additions:
- Unit tests for matcher logic
- Integration tests for scrapers
- E2E tests for frontend
- Mock responses for external dependencies

### Deployment

- Backend: Can be deployed as binary or Docker container
- Frontend: Static export or server-side rendering
- Database: Currently in-memory, consider persistent storage for production
- Reverse proxy: Nginx for load balancing and SSL termination

This instruction file should be referenced when writing AI prompts for:
- Adding new scrapers
- Modifying matching logic
- Enhancing UI components
- Adding new features
- Debugging issues
- Performance optimization
- Code refactoring</content>
<parameter name="filePath">/Users/master/Documents/github/web-scrap/INSTRUCTION.md