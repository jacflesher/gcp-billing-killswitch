package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// PubSubMessage is the wrapper GCP uses for push subscriptions
type PubSubMessage struct {
	Message struct {
		Data []byte `json:"data"`
	} `json:"message"`
}

// BillingPayload is the actual data from the budget alert
type BillingPayload struct {
	Threshold float64 `json:"alertThresholdExceeded"`
}

func main() {
	http.HandleFunc("/", handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %s", err)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	var m PubSubMessage
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		log.Printf("Error decoding: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var data BillingPayload
	if err := json.Unmarshal(m.Message.Data, &data); err != nil {
		log.Printf("Error unmarshaling inner data: %v", err)
		return
	}

	log.Printf("Alert received! Threshold: %.2f", data.Threshold)

	// 1. Get project number ONCE (Env var with Metadata fallback)
	projectNumber := os.Getenv("GCP_PROJECT_NUMBER")
	if projectNumber == "" {
		log.Println("The GCP_PROJECT_NUMBER environment variable is not set. Requesting the information from metadata server...")
		projectNumber = getProjectNumberFromMetadata()
	}

	// 2. Trigger logic
	if data.Threshold >= 1.0 {
		if projectNumber != "" {
			log.Println("CRITICAL: 100% threshold reached. Initiating billing disconnect...")
			disableBilling(projectNumber)
		} else {
			log.Println("ERROR: Threshold reached but Project Number is unknown!")
		}
	}

	w.WriteHeader(http.StatusOK)
}

func getProjectNumberFromMetadata() string {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/project/numeric-project-id", nil)
	req.Header.Add("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(bytes.TrimSpace(body))
}

func disableBilling(projectNumber string) {
	client := &http.Client{}

	// 1. Fallback: If projectNumber is empty, fetch from Metadata Server
	if projectNumber == "" {
		log.Println("DEBUG: projectNumber empty, attempting metadata fetch...")
		req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/project/numeric-project-id", nil)
		req.Header.Add("Metadata-Flavor", "Google")
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			projectNumber = string(bytes.TrimSpace(body))
		}
	}

	if projectNumber == "" {
		log.Println("FAILURE: Could not determine Project Number.")
		return
	}

	// 2. Get Auth Token
	tokenReq, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	tokenReq.Header.Add("Metadata-Flavor", "Google")
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		log.Printf("Token Error: %v", err)
		return
	}
	defer tokenResp.Body.Close()
	var t struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(tokenResp.Body).Decode(&t)

	// 3. Prepare Request
	billingURL := fmt.Sprintf("https://cloudbilling.googleapis.com/v1/projects/%s/billingInfo", projectNumber)
	log.Printf("DEBUG: Executing PUT to [%s]", billingURL)

	// The API expects the resource name and an empty billing account name to unlink
	data := map[string]string{
		"name":               "projects/" + projectNumber + "/billingInfo",
		"billingAccountName": "",
	}
	payload, _ := json.Marshal(data)

	req, _ := http.NewRequest("PUT", billingURL, bytes.NewBuffer(payload))
	req.Header.Add("Authorization", "Bearer "+t.AccessToken)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	// 4. Execute
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Request Error: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		log.Printf("SUCCESS: Project %s unlinked from billing.", projectNumber)
	} else {
		log.Printf("FAILURE: Status %s - Body: %s", resp.Status, string(body))
	}
}
