package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
)

const (
	defaultAddr      = "http://localhost:8765"
	runEndpoint      = "/run"
	testCaseEndpoint = "/testcase"
	clusterEndpoint  = "/cluster"
	Success          = "succeed"
)

type TResponse struct {
	TestID  string      `json:"test_id"`
	Name    string      `json:"name"`
	Status  string      `json:"run_status"`
	Error   string      `json:"error"`
	Details interface{} `json:"details"`
}

func runner(runID string) error {
	URL := fmt.Sprintf("%s%s?id=%s", defaultAddr, runEndpoint, runID)
	resp, err := http.Get(URL)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		res := &TResponse{}

		if err := json.Unmarshal(bodyBytes, res); err != nil {
			return err
		}

		if res.Status != Success {
			return fmt.Errorf("failed test on %s, with status %s err: %s", res.TestID, res.Status, res.Status)
		}

		return nil
	}

	return fmt.Errorf("incorrect response code %v", resp.StatusCode)
}

func isSeverUp() error {
	URL := fmt.Sprintf("%s%s", defaultAddr, clusterEndpoint)
	resp, err := http.Get(URL)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("E2E server is not up")
	}

	return nil
}

func TestE2ESuite(t *testing.T) {
	if err := isSeverUp(); err != nil {
		t.Fatal(err)
	}

	testIDs := []string{"chn-001", "chn-002", "chn-003", "chn-004"}

	for _, tID := range testIDs {
		if err := runner(tID); err != nil {
			t.Fatal(err)
		}
	}

	t.Logf("channel e2e tests %v passed", testIDs)
}
