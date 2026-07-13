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

// Command hrmock is a standalone local stand-in for the WSO2 HR entity
// GraphQL service (hr_entity), used only for local development of the Risk
// module's "Risk Identified By: Employee" feature. It is NOT part of the
// deployed grc-platform backend and does not touch the digiops-hr-main repo
// or any real HR database.
//
// It serves the same wire contract the Go backend's hrentity.Client speaks:
// a POST /graphql endpoint accepting {query, variables} and returning
// {data: {employees: [...]}} shaped exactly like the real service's
// `employees(filter: EmployeeFilter, limit: Int)` query. Because the contract
// is identical, swapping HR_ENTITY_GRAPHQL_URL from this mock to the real
// Choreo-hosted service at deploy time requires no code changes.
//
// Run: go run ./cmd/hrmock
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// employee is a canned fake WSO2 employee. Names mirror the "Identified By"
// people already used in risk_module_data_schema.sql seed data, so the mock
// and the seeded dummy risks tell a consistent story locally.
type employee struct {
	FirstName      string `json:"firstName"`
	LastName       string `json:"lastName"`
	WorkEmail      string `json:"workEmail"`
	EmployeeStatus string `json:"employeeStatus"`
}

var mockEmployees = []employee{
	{FirstName: "Asel", LastName: "Fernando", WorkEmail: "asel.fernando@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Dineth", LastName: "Perera", WorkEmail: "dineth.perera@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Nimal", LastName: "Jayasinghe", WorkEmail: "nimal.jayasinghe@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Sachini", LastName: "Wijeratne", WorkEmail: "sachini.wijeratne@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Dilini", LastName: "Rathnayake", WorkEmail: "dilini.rathnayake@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Kasun", LastName: "Bandara", WorkEmail: "kasun.bandara@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Ruwan", LastName: "De Silva", WorkEmail: "ruwan.desilva@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Tharushi", LastName: "Mendis", WorkEmail: "tharushi.mendis@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Priya", LastName: "Gunasekara", WorkEmail: "priya.gunasekara@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Ishan", LastName: "Fernando", WorkEmail: "ishan.fernando@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Lahiru", LastName: "Perera", WorkEmail: "lahiru.perera@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Sanduni", LastName: "Kumari", WorkEmail: "sanduni.kumari@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Chamara", LastName: "Wickramasinghe", WorkEmail: "chamara.wickramasinghe@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Amaya", LastName: "Seneviratne", WorkEmail: "amaya.seneviratne@wso2.com", EmployeeStatus: "Active"},
	{FirstName: "Ruwan", LastName: "De Silva Jr", WorkEmail: "ruwan.desilvajr@wso2.com", EmployeeStatus: "left"},
}

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type employeesResponse struct {
	Data struct {
		Employees []employee `json:"employees"`
	} `json:"data"`
}

func handleGraphQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req graphqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	emailSearchString, _ := digStringFilter(req.Variables, "filter", "emailSearchString")
	statuses := digStatusFilter(req.Variables)

	var matched []employee
	for _, e := range mockEmployees {
		if emailSearchString != "" && !strings.Contains(strings.ToLower(e.WorkEmail), strings.ToLower(emailSearchString)) {
			continue
		}
		if len(statuses) > 0 && !containsFold(statuses, e.EmployeeStatus) {
			continue
		}
		matched = append(matched, e)
	}

	var resp employeesResponse
	resp.Data.Employees = matched

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// digStringFilter reads variables["filter"][key] as a string, if present.
func digStringFilter(variables map[string]any, filterKey, fieldKey string) (string, bool) {
	filter, ok := variables[filterKey].(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := filter[fieldKey].(string)
	return v, ok
}

// digStatusFilter reads variables["filter"]["employeeStatus"] as a []string, if present.
func digStatusFilter(variables map[string]any) []string {
	filter, ok := variables["filter"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := filter["employeeStatus"].([]any)
	if !ok {
		return nil
	}
	statuses := make([]string, 0, len(raw))
	for _, s := range raw {
		if str, ok := s.(string); ok {
			statuses = append(statuses, str)
		}
	}
	return statuses
}

func containsFold(list []string, target string) bool {
	for _, s := range list {
		if strings.EqualFold(s, target) {
			return true
		}
	}
	return false
}

func main() {
	// Deliberately not "PORT" — cmd/server's .env already sets PORT=:8080,
	// and both get sourced from the same file in local dev.
	port := os.Getenv("HRMOCK_PORT")
	if port == "" {
		port = "9090"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", handleGraphQL)

	log.Printf("hr entity mock server listening on :%s (POST /graphql)", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil { //nolint:gosec
		log.Fatal(err)
	}
}
