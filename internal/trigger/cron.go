package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/peiblow/eeapi/internal/schema"
	"github.com/peiblow/eeapi/internal/service"
	"github.com/robfig/cron/v3"
)

type CronProvider struct {
	cron *cron.Cron
}

func NewCronProvider() *CronProvider {
	return &CronProvider{
		cron: cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger))),
	}
}

func (c *CronProvider) Type() string { return "cron" }

func (c *CronProvider) Start(events service.EventService, bindings []Binding) error {
	registered := 0
	for _, b := range bindings {
		if err := c.register(events, b); err != nil {
			slog.Error("skipping cron trigger", "agent", b.Agent.Hash, "error", err)
			continue
		}
		registered++
	}

	c.cron.Start()
	slog.Info("cron provider started", "triggers", registered)
	return nil
}

func (c *CronProvider) Stop() {
	c.cron.Stop()
}

func (c *CronProvider) register(events service.EventService, b Binding) error {
	schedule, _ := b.Config["schedule"].(string)
	if err := validateCronSchedule(schedule); err != nil {
		return err
	}

	timezone, _ := b.Config["timezone"].(string)
	if timezone == "" {
		timezone = "UTC"
	}

	skillName, _ := b.Config["skill"].(string)
	text, err := resolveSkillText(b.Agent, skillName)
	if err != nil {
		return err
	}

	spec := "CRON_TZ=" + timezone + " " + schedule
	agentHash := b.Agent.Hash
	source := "cron:" + skillName

	_, err = c.cron.AddFunc(spec, func() {
		fire(events, agentHash, source, text)
	})
	return err
}

func fire(events service.EventService, agentHash, source, text string) {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		slog.Error("cron trigger marshal failed", "agent", agentHash, "source", source, "error", err)
		return
	}

	if _, err := events.EnqueueAgentEvent(context.Background(), agentHash, &service.EnqueueEventInput{
		ContextID: uuid.New().String(),
		Source:    source,
		Payload:   payload,
	}); err != nil {
		slog.Error("cron trigger enqueue failed", "agent", agentHash, "source", source, "error", err)
		return
	}
	slog.Info("cron trigger fired", "agent", agentHash, "source", source)
}

func resolveSkillText(agent schema.AgentDefinition, skillName string) (string, error) {
	if skillName == "" {
		return "", fmt.Errorf("cron trigger has no skill link")
	}
	for _, s := range agent.Skills {
		if s.Name == skillName {
			if s.Content == "" {
				return "", fmt.Errorf("skill %q has empty content", skillName)
			}
			return s.Content, nil
		}
	}
	return "", fmt.Errorf("skill %q not found in agent %s", skillName, agent.Hash)
}

func validateCronSchedule(schedule string) error {
	if schedule == "" {
		return fmt.Errorf("cron trigger has no schedule")
	}
	if len(strings.Fields(schedule)) != 5 {
		return fmt.Errorf("cron schedule must have 5 fields (min hour dom month dow), got %q", schedule)
	}
	if _, err := cron.ParseStandard(schedule); err != nil {
		return fmt.Errorf("invalid cron schedule %q: %w", schedule, err)
	}
	return nil
}
