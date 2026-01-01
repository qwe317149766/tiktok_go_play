package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// compareWithJS runs the JavaScript version and compares output
// This requires Node.js to be installed
func compareWithJS(jsFile, functionName string, args []interface{}) (string, error) {
	// Create a temporary JS file to call the function
	jsCode := fmt.Sprintf(`
const mod = require('./%s');
const args = %s;
const result = mod.apply(null, args);
console.log(JSON.stringify({result: result}));
`, jsFile, toJSONArray(args))

	tmpFile := "/tmp/test_compare.js"
	err := os.WriteFile(tmpFile, []byte(jsCode), 0644)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile)

	cmd := exec.Command("node", tmpFile)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("node execution failed: %v, output: %s", err, string(output))
	}

	var result struct {
		Result string `json:"result"`
	}
	err = json.Unmarshal(output, &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse JSON: %v", err)
	}

	return result.Result, nil
}

func toJSONArray(args []interface{}) string {
	jsonBytes, _ := json.Marshal(args)
	return string(jsonBytes)
}

func TestXbogusCompareWithJS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping JavaScript comparison test in short mode")
	}

	// Check if node is available
	_, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js not found, skipping comparison test")
	}

	testCases := []struct {
		name      string
		params    string
		postData  string
		userAgent string
		timestamp uint32
	}{
		{
			name:      "basic test",
			params:    "device_platform=android&os=android",
			postData:  "{}",
			userAgent: "Mozilla/5.0",
			timestamp: 1234567890,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Go version
			goResult := Encrypt(tc.params, tc.postData, tc.userAgent, tc.timestamp)

			// JavaScript version
			jsArgs := []interface{}{tc.params, tc.postData, tc.userAgent, tc.timestamp}
			jsResult, err := compareWithJS("xbogus.js", "encrypt", jsArgs)
			if err != nil {
				t.Fatalf("Failed to run JavaScript version: %v", err)
			}

			if goResult != jsResult {
				t.Errorf("Output mismatch!\nGo:      %s\nJavaScript: %s", goResult, jsResult)
			} else {
				t.Logf("Outputs match: %s", goResult)
			}
		})
	}
}

func TestXgnarlyCompareWithJS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping JavaScript comparison test in short mode")
	}

	// Check if node is available
	_, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js not found, skipping comparison test")
	}

	testCases := []struct {
		name        string
		queryString string
		body        string
		userAgent   string
		envcode     int
		version     string
		timestampMs int64
	}{
		{
			name:        "basic test",
			queryString: "device_platform=android&os=android",
			body:        "{}",
			userAgent:   "Mozilla/5.0",
			envcode:     0,
			version:     "5.1.1",
			timestampMs: 1234567890000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Go version
			goResult, err := EncryptXgnarly(tc.queryString, tc.body, tc.userAgent, tc.envcode, tc.version, tc.timestampMs)
			if err != nil {
				t.Fatalf("Go version failed: %v", err)
			}

			// JavaScript version
			jsArgs := []interface{}{tc.queryString, tc.body, tc.userAgent, tc.envcode, tc.version, tc.timestampMs}
			jsResult, err := compareWithJS("xgnarly.js", "encrypt", jsArgs)
			if err != nil {
				t.Fatalf("Failed to run JavaScript version: %v", err)
			}

			if goResult != jsResult {
				t.Errorf("Output mismatch!\nGo:      %s\nJavaScript: %s", goResult, jsResult)
			} else {
				t.Logf("Outputs match: %s", goResult)
			}
		})
	}
}

