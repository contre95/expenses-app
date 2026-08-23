package ollama

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

type OllamaAPI struct {
	TimeOut     time.Duration
	apiURL      string
	client      *http.Client
	txtModel    string
	visionModel string
}

type ExpenseGuess struct {
	Shop        string
	Amount      float64
	Date        time.Time
	Description string
	Product     string
}

var OUTPUT_PROMPT = `1. OUTPUT FORMAT:
{
    "transactions": [
        {
            "shop": "Exact Merchant Name", 
            "amount": 123.45,
            "date": "YYYY-MM-DD",
            "description": "Product or service description"
        },
      ...
    ]
}
2. FIELD REQUIREMENTS:
- shop: String containing ONLY the business name (no lists/arrays)
- amount: POSITIVE float (convert negatives to positive, NO symbols)
- date: Full date in STRICT YYYY-MM-DD format (ignore time if present)
- description: Optional short description of the product/service from the transaction
- if not description then use AI Generated.
3. STRICT PROHIBITIONS:
- NO plural field names (use "shop", not "shops")
- NO arrays in values (single value per field)
- NO negative amounts
- NO time components in dates
- NO currency symbols (€/$) or special characters
4. DATA CLEANSING:
- If date can't be converted to YYYY-MM-DD, OMIT ENTIRE ENTRY
- If amount contains symbols, REMOVE THEM (keep numeric value)
- If amount is negative, CONVERT TO POSITIVE
- If description is missing, empty, or only whitespace, use AI Generated
Convert this transaction text to JSON. Follow ALL rules EXACTLY.
    ` + "If no specific year is specified, assume is " + time.Now().Format("2006") + `
    ` + "If no specific month is specified, assume is " + time.Now().Format("Jan") + `
    ` + "Treat today as day " + time.Now().Format("02") + "and yesterday as " + time.Now().Add(-24*time.Hour).Format("02") + `
GOOD EXAMPLE:
{
    "transactions": [
        {"shop": "ShopName1", "amount": 65.00, "date": "YYYY-MM-DD", "description": "Coffee"},
        {"shop": "ShopName2", "amount": 5.00, "date": "YYYY-MM-DD", "description": "AI Generated"},
        ...
        {"shop": "ShopName8", "amount": 1.78, "date": "YYYY-MM-DD", "description": "AI Generated"}
    ]
}
`

// const TEXT_MODEL = "llama3.2:3b-instruct-q4_K_M"

func NewOllamaAPI(txtModel, imgModel, ollamaEndpoint string, to time.Duration) (*OllamaAPI, error) {
	return &OllamaAPI{
		TimeOut:     to,
		client:      &http.Client{Timeout: to},
		apiURL:      ollamaEndpoint,
		txtModel:    txtModel,
		visionModel: imgModel,
	}, nil
}

// TODO: Implement context cancelation for different timeout.

func (o *OllamaAPI) IsRunning(ctx context.Context) (bool, error) {
	healthEndpoint := o.apiURL
	// Create a new HTTP request with the provided context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthEndpoint, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	// Perform the HTTP request
	resp, err := o.client.Do(req)
	if err != nil {
		// Check if the error is due to context cancellation or timeout
		if ctx.Err() == context.Canceled {
			return false, fmt.Errorf("request canceled: %w", ctx.Err())
		} else if ctx.Err() == context.DeadlineExceeded {
			return false, fmt.Errorf("request timed out: %w", ctx.Err())
		}
		return false, fmt.Errorf("failed to reach Ollama API: %w", err)
	}
	defer resp.Body.Close()
	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Ollama API returned non-OK status: %s", resp.Status)
	}
	return true, nil
}

// Shared request handling
func (o *OllamaAPI) sendRequest(jsonBody []byte) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "POST", o.apiURL+"/api/generate", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status: %d", resp.StatusCode)
	}

	var apiResponse struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return "", fmt.Errorf("error decoding response: %v", err)
	}

	return apiResponse.Response, nil
}

// Shared parsing logic
func (o *OllamaAPI) parseAndConvertTransactions(responseStr string) ([]ExpenseGuess, error) {
	fmt.Printf("\n%s\n", responseStr)
	var response struct {
		Transactions []struct {
			Date        string  `json:"date"`
			Shop        string  `json:"shop"`
			Amount      float64 `json:"amount"`
			Description string  `json:"description"`
		} `json:"transactions"`
	}

	if err := json.Unmarshal([]byte(responseStr), &response); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %v", err)
	}

	guesses := make([]ExpenseGuess, 0, len(response.Transactions))
	for _, tx := range response.Transactions {
		parsedDate, err := parseDate(tx.Date)
		if err != nil {
			return nil, fmt.Errorf("error parsing date %q: %v", tx.Date, err)
		}

		description := strings.TrimSpace(tx.Description)
		if description == "" {
			description = "AI Generated"
		}

		guesses = append(guesses, ExpenseGuess{
			Shop:        tx.Shop,
			Amount:      math.Abs(tx.Amount),
			Date:        parsedDate,
			Description: description,
			Product:     description,
		})
	}

	if len(guesses) == 0 {
		return nil, fmt.Errorf("no valid transactions found in response")
	}

	return guesses, nil
}

func parseDate(dateStr string) (time.Time, error) {
	dateFormats := []string{
		"02/01/2006",
		"02/01/06",
		"2006-01-02",
		"01/02/2006",
		"02-01-2006",
		"Jan 2, 2006",
		"02 Jan 2006",
		"02/01/2006 3:04 PM",
		"02/01/2006 15:04",
	}

	for _, format := range dateFormats {
		parsedDate, err := time.Parse(format, dateStr)
		if err == nil {
			return parsedDate, nil
		}
	}

	return time.Time{}, fmt.Errorf("error parsing date %q: no matching format found", dateStr)
}

func (o *OllamaAPI) GuessFromImage(imageData []byte) ([]ExpenseGuess, error) {
	encodedImage := base64.StdEncoding.EncodeToString(imageData)
	requestBody := map[string]interface{}{
		"model": o.visionModel,
		"prompt": `You are an image to json model converter of bank transaction screenshots. Convert the information to JSON following these STRICT RULES:
    ` + OUTPUT_PROMPT + `
Current screenshot to parse:`,
		"stream": false,
		"format": "json",
		"images": []string{encodedImage},
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error encoding request: %v", err)
	}

	resp, err := o.sendRequest(jsonBody)
	if err != nil {
		return nil, err
	}

	fmt.Printf("\n\n%s\n\n", resp)
	return o.parseAndConvertTransactions(resp)
}

func (o *OllamaAPI) GuessFromText(text string) ([]ExpenseGuess, error) {
	prompt := fmt.Sprintf(`You are a text to JSON model converter of bank transaction descriptions. Convert the information to JSON following these STRICT RULES:
You are a text to JSON model converter of bank transaction descriptions. Convert the information to JSON following these STRICT RULES:
    `+OUTPUT_PROMPT+`
Current  text to parse: %s`, text)
	requestBody := map[string]interface{}{
		"model":  o.txtModel,
		"prompt": prompt,
		"stream": false,
		"format": "json",
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error encoding request: %v", err)
	}

	resp, err := o.sendRequest(jsonBody)
	if err != nil {
		return nil, err
	}

	return o.parseAndConvertTransactions(resp)
}
