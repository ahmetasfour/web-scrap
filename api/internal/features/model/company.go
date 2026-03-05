package model

// Company represents a row from the German property management Excel file.
type Company struct {
	ID             int    `json:"id"`
	EnObjekt       int    `json:"enObjekt"`
	ReName         string `json:"reName"`
	ReName2        string `json:"reName2"`
	ObjektRechnung string `json:"objektRechnung"`
	ReOrt          string `json:"reOrt"`
	ReHausnummer   string `json:"reHausnummer"`
	RePlz          string `json:"rePlz"`
	ReStrasse      string `json:"reStrasse"`
	ReNummer       string `json:"reNummer"`
	Email          string `json:"email"`
	Telefonnummer  string `json:"telefonnummer"`
}

// ScrapeResult embeds Company and adds scraped contact data.
type ScrapeResult struct {
	Company
	Status string   `json:"status"` // "done", "not_found", "error"
	Emails []string `json:"emails"`
	Phones []string `json:"phones"`
	Source string   `json:"source"`
	Error  string   `json:"error,omitempty"`
}

// ScrapeRequest is the JSON body for POST /api/scrape.
type ScrapeRequest struct {
	Companies []Company `json:"companies"`
}

// ScrapeResponse is the JSON body returned by POST /api/scrape.
type ScrapeResponse struct {
	Results []ScrapeResult `json:"results"`
}
