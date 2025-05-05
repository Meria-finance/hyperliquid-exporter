package monitors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/validaoxyz/hyperliquid-exporter/internal/config"
	"github.com/validaoxyz/hyperliquid-exporter/internal/logger"
	"github.com/validaoxyz/hyperliquid-exporter/internal/metrics"
)

type ValidatorSummary struct {
	Validator       string  `json:"validator"`
	Signer          string  `json:"signer"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	NRecentBlocks   int     `json:"nRecentBlocks"`
	Stake           float64 `json:"stake"`
	IsJailed        bool    `json:"isJailed"`
	UnjailableAfter int64   `json:"unjailableAfter"`
	IsActive        bool    `json:"isActive"`
}

func StartValidatorMonitor(ctx context.Context, cfg config.Config, errCh chan<- error) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := updateValidatorMetrics(ctx, cfg); err != nil {
					logger.Error("Validator monitor error: %v", err)
					errCh <- err
				}
			}
		}
	}()
}

func updateValidatorMetrics(ctx context.Context, cfg config.Config) error {
	client := &http.Client{Timeout: 10 * time.Second}
	
	// Déterminer l'URL de l'API en fonction de la chaîne
	var apiURL string
	if cfg.Chain == "mainnet" {
		apiURL = "https://api.hyperliquid.xyz/info"
	} else {
		apiURL = "https://api.hyperliquid-testnet.xyz/info"
	}
	
	payload := []byte(`{"type": "validatorSummaries"}`)

	logger.Debug("Making request to validator API")

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	var summaries []ValidatorSummary

	if err := json.Unmarshal(body, &summaries); err != nil {
		logger.Debug("Unmarshal error occurred here in validator_api_monitor.go")
		return fmt.Errorf("error unmarshaling response: %w", err)
	}

	totalStake := 0.0
	jailedStake := 0.0
	notJailedStake := 0.0
	activeStake := 0.0
	inactiveStake := 0.0

	for _, summary := range summaries {
		metrics.SetValidatorStake(summary.Validator, summary.Signer, summary.Name, summary.Stake)

		// Update validator jailed status
		jailedStatus := 0.0
		if summary.IsJailed {
			jailedStatus = 1.0
			jailedStake += summary.Stake
		} else {
			notJailedStake += summary.Stake
		}
		metrics.SetValidatorJailedStatus(summary.Validator, summary.Signer, summary.Name, jailedStatus)

		// Update active/inactive stake
		if summary.IsActive {
			activeStake += summary.Stake
		} else {
			inactiveStake += summary.Stake
		}

		// Set active status
		activeStatus := 0.0
		if summary.IsActive {
			activeStatus = 1.0
		}
		metrics.SetValidatorActiveStatus(summary.Validator, summary.Signer, summary.Name, activeStatus)

		totalStake += summary.Stake
	}

	// Update aggregate metrics
	metrics.SetTotalStake(totalStake)
	metrics.SetJailedStake(jailedStake)
	metrics.SetNotJailedStake(notJailedStake)
	metrics.SetActiveStake(activeStake)
	metrics.SetInactiveStake(inactiveStake)
	metrics.SetValidatorCount(int64(len(summaries)))

	logger.Info("Updated validator metrics: Total validators: %d", len(summaries))
	logger.Info("Total stake: %f, Jailed stake: %f, Not jailed stake: %f, Active stake: %f, Inactive stake: %f",
		totalStake, jailedStake, notJailedStake, activeStake, inactiveStake)

	return nil
}
