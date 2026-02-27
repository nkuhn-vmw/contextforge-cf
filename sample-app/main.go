package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type vcapServices map[string][]struct {
	Credentials struct {
		URL      string `json:"url"`
		MCPURL   string `json:"mcp_url"`
		Username string `json:"username"`
		JWTToken string `json:"jwt_token"`
		URI      string `json:"uri"`
	} `json:"credentials"`
	Name string `json:"name"`
}

var creds struct {
	URL      string
	MCPURL   string
	Username string
	JWTToken string
	Bound    bool
}

func init() {
	raw := os.Getenv("VCAP_SERVICES")
	if raw == "" {
		log.Println("VCAP_SERVICES not set — running without ContextForge binding")
		return
	}

	var services vcapServices
	if err := json.Unmarshal([]byte(raw), &services); err != nil {
		log.Printf("Failed to parse VCAP_SERVICES: %v", err)
		return
	}

	bindings, ok := services["contextforge-mcp-gateway"]
	if !ok || len(bindings) == 0 {
		log.Println("No contextforge-mcp-gateway binding found in VCAP_SERVICES")
		return
	}

	c := bindings[0].Credentials
	creds.URL = c.URL
	creds.MCPURL = c.MCPURL
	creds.Username = c.Username
	creds.JWTToken = c.JWTToken
	creds.Bound = true
	log.Printf("ContextForge binding found: url=%s username=%s", c.URL, c.Username)
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/test", testHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Sample app starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>ContextForge Sample App</title></head>
<body>
<h1>ContextForge MCP Gateway — Sample App</h1>
<p><strong>Bound:</strong> %v</p>
<p><strong>URL:</strong> %s</p>
<p><strong>MCP URL:</strong> %s</p>
<p><strong>Username:</strong> %s</p>
<p><strong>JWT Token:</strong> %s...</p>
<hr>
<p><a href="/test">Run validation tests</a></p>
</body></html>`, creds.Bound, creds.URL, creds.MCPURL, creds.Username, truncate(creds.JWTToken, 40))
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !creds.Bound {
		json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"error":  "No contextforge-mcp-gateway binding found",
		})
		return
	}

	results := make(map[string]any)
	results["binding"] = map[string]any{
		"url":      creds.URL,
		"mcp_url":  creds.MCPURL,
		"username": creds.Username,
		"has_jwt":  creds.JWTToken != "",
	}

	// Test 1: Health check without auth
	healthStatus, healthBody, err := httpGet(creds.URL+"/health", "")
	results["health_no_auth"] = map[string]any{
		"status": healthStatus,
		"body":   healthBody,
		"pass":   err == nil && healthStatus == 200,
	}

	// Test 2: Health check with JWT auth
	healthStatus2, healthBody2, err2 := httpGet(creds.URL+"/health", creds.JWTToken)
	results["health_with_jwt"] = map[string]any{
		"status": healthStatus2,
		"body":   healthBody2,
		"pass":   err2 == nil && healthStatus2 == 200,
	}

	// Test 3: MCP endpoint with JWT auth (expect some response, not 401)
	mcpStatus, mcpBody, err3 := httpGet(creds.MCPURL, creds.JWTToken)
	results["mcp_with_jwt"] = map[string]any{
		"status": mcpStatus,
		"body":   truncate(mcpBody, 200),
		"pass":   err3 == nil && mcpStatus != 401 && mcpStatus != 403,
	}

	// Overall pass/fail
	allPass := results["health_no_auth"].(map[string]any)["pass"].(bool) &&
		results["health_with_jwt"].(map[string]any)["pass"].(bool) &&
		results["mcp_with_jwt"].(map[string]any)["pass"].(bool)

	results["overall"] = map[string]any{
		"pass":    allPass,
		"message": passMessage(allPass),
	}

	json.NewEncoder(w).Encode(results)
}

func httpGet(url, token string) (int, string, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func passMessage(pass bool) string {
	if pass {
		return "All tests passed"
	}
	return "Some tests failed"
}
