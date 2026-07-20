// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"math"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// DashboardService assembles the risk dashboard payload.
type DashboardService interface {
	// Summary builds the dashboard payload, optionally scoped to one
	// register (nil = all registers).
	Summary(ctx context.Context, registerID *int) (*model.DashboardSummary, error)
}

type dashboardService struct {
	repo repository.DashboardRepository
}

// NewDashboardService creates a DashboardService backed by repo.
func NewDashboardService(repo repository.DashboardRepository) DashboardService {
	return &dashboardService{repo: repo}
}

// riskLevelOrder is only referenced by analytics.go now — dashboard.go's own
// level-based charts source their order from the DB via LevelOrder instead.
// Left in place because analytics.go's own hardcoded-level-list fix already
// landed on a separate branch and will replace this usage on merge.
var riskLevelOrder = []string{"HIGH", "MEDIUM", "LOW"}

// statusBucketOrder fixes the x-axis order of each register's status chart.
var statusBucketOrder = []string{"CLOSED", "REMEDIATE", "ACCEPT", "TRANSFER", "VOID"}

func (s *dashboardService) Summary(ctx context.Context, registerID *int) (*model.DashboardSummary, error) {
	counts, err := s.repo.StatusCounts(ctx, registerID)
	if err != nil {
		return nil, err
	}
	facts, err := s.repo.OpenRiskFacts(ctx, registerID)
	if err != nil {
		return nil, err
	}
	statusFacts, err := s.repo.RegisterStatusFacts(ctx, registerID)
	if err != nil {
		return nil, err
	}
	certCounts, err := s.repo.CertTagCounts(ctx, registerID)
	if err != nil {
		return nil, err
	}
	repeatedRows, err := s.repo.RepeatedComplianceRisks(ctx, registerID)
	if err != nil {
		return nil, err
	}
	highRisks, err := s.repo.HighRisks(ctx, registerID)
	if err != nil {
		return nil, err
	}
	if highRisks == nil {
		highRisks = []model.HighRiskItem{}
	}
	levelOrder, err := s.repo.LevelOrder(ctx)
	if err != nil {
		return nil, err
	}

	return &model.DashboardSummary{
		Summary:                 *counts,
		TreatmentByRegister:     buildTreatmentByRegister(facts),
		LevelCounts:             buildLevelCounts(facts, levelOrder),
		OrgHeatmap:              buildHeatmap(facts),
		CertDistribution:        buildCertDistribution(certCounts),
		Registers:               buildRegisterBlocks(facts, statusFacts, levelOrder),
		RepeatedComplianceRisks: buildRepeatedRisks(repeatedRows),
		HighRisks:               highRisks,
	}, nil
}

// buildTreatmentByRegister collapses facts into register × treatment counts,
// preserving the repository's register-name ordering.
func buildTreatmentByRegister(facts []model.OpenRiskFact) []model.RegisterTreatmentCount {
	type key struct{ register, strategy string }
	counts := map[key]int{}
	var order []key
	for _, f := range facts {
		k := key{f.RegisterName, f.TreatmentStrategy}
		if _, seen := counts[k]; !seen {
			order = append(order, k)
		}
		counts[k] += f.Count
	}
	out := make([]model.RegisterTreatmentCount, 0, len(order))
	for _, k := range order {
		out = append(out, model.RegisterTreatmentCount{
			RegisterName:      k.register,
			TreatmentStrategy: k.strategy,
			Count:             counts[k],
		})
	}
	return out
}

// buildLevelCounts collapses facts into per-level totals, ordered by
// levelOrder (severity, highest first — sourced from risk_score so a level
// added there is picked up automatically instead of being silently dropped).
func buildLevelCounts(facts []model.OpenRiskFact, levelOrder []string) []model.RiskLevelCount {
	counts := map[string]int{}
	colors := map[string]string{}
	for _, f := range facts {
		counts[f.RiskLevel] += f.Count
		colors[f.RiskLevel] = f.ColorCode
	}
	out := make([]model.RiskLevelCount, 0, len(levelOrder))
	for _, level := range levelOrder {
		if n, ok := counts[level]; ok {
			out = append(out, model.RiskLevelCount{RiskLevel: level, ColorCode: colors[level], Count: n})
		}
	}
	return out
}

// buildHeatmap collapses facts into likelihood × impact cell counts.
// Only populated cells are returned; the frontend renders the full 3×3 grid.
func buildHeatmap(facts []model.OpenRiskFact) []model.HeatmapCell {
	type key struct{ likelihood, impact int }
	cells := map[key]*model.HeatmapCell{}
	var order []key
	for _, f := range facts {
		k := key{f.Likelihood, f.Impact}
		if c, ok := cells[k]; ok {
			c.Count += f.Count
			continue
		}
		cells[k] = &model.HeatmapCell{
			Likelihood: f.Likelihood,
			Impact:     f.Impact,
			RiskLevel:  f.RiskLevel,
			ColorCode:  f.ColorCode,
			Count:      f.Count,
		}
		order = append(order, k)
	}
	out := make([]model.HeatmapCell, 0, len(order))
	for _, k := range order {
		out = append(out, *cells[k])
	}
	return out
}

// buildCertDistribution converts cert-tag counts into each certification's
// percentage share of its register's tags (segments per register total 100%).
func buildCertDistribution(counts []model.RegisterCertCount) []model.RegisterCertShare {
	totals := map[string]int{}
	for _, c := range counts {
		totals[c.RegisterName] += c.Count
	}
	out := make([]model.RegisterCertShare, 0, len(counts))
	for _, c := range counts {
		pct := float64(c.Count) * 100 / float64(totals[c.RegisterName])
		out = append(out, model.RegisterCertShare{
			RegisterName: c.RegisterName,
			CertName:     c.CertName,
			Count:        c.Count,
			Percentage:   math.Round(pct*10) / 10,
		})
	}
	return out
}

// buildRegisterBlocks groups facts into one dashboard section per register.
// statusFacts (open + closed) is the superset that determines which
// registers appear and their order; facts (open-only) supplements OpenCount
// and the heatmap, which stay open-scoped.
func buildRegisterBlocks(facts []model.OpenRiskFact, statusFacts []model.RegisterStatusFact, levelOrder []string) []model.RegisterAnalytics {
	blocks := map[int]*model.RegisterAnalytics{}
	var order []int

	statusGrouped := map[int][]model.RegisterStatusFact{}
	for _, f := range statusFacts {
		if _, ok := blocks[f.RegisterID]; !ok {
			blocks[f.RegisterID] = &model.RegisterAnalytics{
				RegisterID:   f.RegisterID,
				RegisterName: f.RegisterName,
			}
			order = append(order, f.RegisterID)
		}
		statusGrouped[f.RegisterID] = append(statusGrouped[f.RegisterID], f)
	}

	grouped := map[int][]model.OpenRiskFact{}
	for _, f := range facts {
		blocks[f.RegisterID].OpenCount += f.Count
		grouped[f.RegisterID] = append(grouped[f.RegisterID], f)
	}

	out := make([]model.RegisterAnalytics, 0, len(order))
	for _, id := range order {
		b := blocks[id]
		b.Heatmap = buildHeatmap(grouped[id])
		b.StatusLevels = buildStatusLevels(statusGrouped[id], levelOrder)
		out = append(out, *b)
	}
	return out
}

// buildStatusLevels collapses one register's status facts into bucket ×
// level counts, ordered by statusBucketOrder then levelOrder (severity).
func buildStatusLevels(facts []model.RegisterStatusFact, levelOrder []string) []model.RegisterStatusLevelCount {
	type key struct{ bucket, level string }
	counts := map[key]int{}
	colors := map[string]string{}
	for _, f := range facts {
		counts[key{f.Bucket, f.RiskLevel}] += f.Count
		colors[f.RiskLevel] = f.ColorCode
	}
	var out []model.RegisterStatusLevelCount
	for _, bucket := range statusBucketOrder {
		for _, level := range levelOrder {
			if n, ok := counts[key{bucket, level}]; ok {
				out = append(out, model.RegisterStatusLevelCount{
					Bucket:    bucket,
					RiskLevel: level,
					ColorCode: colors[level],
					Count:     n,
				})
			}
		}
	}
	return out
}

// buildRepeatedRisks groups per-register occurrences under their shared title,
// preserving the repository's title ordering.
func buildRepeatedRisks(rows []model.RepeatedRiskRow) []model.RepeatedComplianceRisk {
	grouped := map[string]*model.RepeatedComplianceRisk{}
	var order []string
	for _, r := range rows {
		g, ok := grouped[r.RiskTitle]
		if !ok {
			g = &model.RepeatedComplianceRisk{RiskTitle: r.RiskTitle}
			grouped[r.RiskTitle] = g
			order = append(order, r.RiskTitle)
		}
		g.Occurrences = append(g.Occurrences, model.RepeatedRiskOccurrence{
			RegisterName: r.RegisterName,
			Status:       r.Status,
			RiskLevel:    r.RiskLevel,
			ColorCode:    r.ColorCode,
		})
	}
	out := make([]model.RepeatedComplianceRisk, 0, len(order))
	for _, title := range order {
		out = append(out, *grouped[title])
	}
	return out
}
