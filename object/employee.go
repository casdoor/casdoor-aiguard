// Copyright 2025 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package object

import "strings"

// A digital employee is the signed-in person seen as a policy subject: the same
// Casbin triple the Policy Hub uses, but written about what *this human* is
// entitled to rather than what one agent may do. The two are meant to be fused
// - an agent acting for an employee may only do what both of them allow.
//
// The set lives on the Casdoor user, in three of its "properties" entries, so
// it follows the person to whatever machine they sign in on rather than
// belonging to one aiguard installation the way the Policy Hub's files do.
const (
	EmployeeModelProperty   = "aiguardModel"
	EmployeePolicyProperty  = "aiguardPolicy"
	EmployeeRequestProperty = "aiguardRequest"
)

// EmployeePolicySet is one person's policy set, plus who it belongs to. It is
// deliberately the same three text blocks a PolicySet carries, so the Web UI
// edits, enforces and fuses both with one set of code.
type EmployeePolicySet struct {
	// Owner and Name are the Casdoor user this set is stored on; Subject is the
	// name the policy's rules are written about, i.e. the "sub" column.
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Subject     string `json:"subject"`

	Model   Text `json:"model"`
	Policy  Text `json:"policy"`
	Request Text `json:"request"`

	// Stored is false while the user has never saved a set, i.e. while what the
	// UI shows is the starter template rather than their own rules.
	Stored bool `json:"stored"`
}

// employeeModelTemplate is the same model every shipped policy set uses, so a
// person's set and an agent's set can be fused without reconciling two
// different request shapes.
const employeeModelTemplate = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p_eft == allow)) && !some(where (p_eft == deny))

[matchers]
m = r.sub == p.sub && regexMatch(r.obj, p.obj) && regexMatch(r.act, p.act)`

// employeePolicyTemplate is the starter set a person gets before they have
// written their own: the entitlements an office worker plausibly has, spelled
// out so the first thing they see is editable rather than empty. {sub} is
// replaced with their own user name.
const employeePolicyTemplate = `# Your digital employee: what *you* are entitled to do through any AI agent.
# aiguard evaluates one triple per intercepted call:
#   sub = you, obj = the destination host ("host#tool" for an MCP tools/call),
#   act = the intent category the recognizer produced.
# Fusing this with an agent's policy set yields what that agent may do for you.

# 1. The model providers you are cleared to send prompts to.
p, {sub}, ^(.+\.)?(anthropic\.com|openai\.com|googleapis\.com)$, llm\.chat, allow

# 2. Reading and writing your own working tree through a local MCP server.
p, {sub}, .*#(read_file|read_text_file|list_directory|search_files|get_file_info)$, mcp\.tool_call, allow
p, {sub}, .*#(write_file|edit_file|create_file|apply_patch|create_directory)$, mcp\.tool_call, allow
p, {sub}, .*#git_(status|diff|log|show|branch|add|commit)$, mcp\.tool_call, allow

# 3. Publishing on your behalf is yours to do, not an agent's.
p, {sub}, .*#(git_push|git_force_push|delete_file|delete_directory)$, mcp\.tool_call, deny

# 4. Your credentials are never an agent's to read.
p, {sub}, .*#.*(secret|credential|token|password|api_key|keychain|env_file).*$, mcp\.tool_call, deny

# 5. You do not spend money through an agent.
p, {sub}, .*, payment, deny`

// employeeRequestTemplate is a handful of example calls covering each rule
// above, so the enforcement result below the editors is never empty.
const employeeRequestTemplate = `{sub}, api.anthropic.com, llm.chat
{sub}, api.deepseek.com, llm.chat
{sub}, 127.0.0.1:3000#read_file, mcp.tool_call
{sub}, 127.0.0.1:3000#edit_file, mcp.tool_call
{sub}, 127.0.0.1:3000#git_commit, mcp.tool_call
{sub}, 127.0.0.1:3000#git_push, mcp.tool_call
{sub}, 127.0.0.1:3000#read_env_file, mcp.tool_call
{sub}, api.stripe.com, payment`

// DefaultEmployeePolicySet builds the starter set for a person who has never
// saved one, with every rule written about them by name.
func DefaultEmployeePolicySet(subject string) (model Text, policy Text, request Text) {
	fill := func(template string) Text {
		return Text(strings.ReplaceAll(template, "{sub}", subject))
	}
	return Text(employeeModelTemplate), fill(employeePolicyTemplate), fill(employeeRequestTemplate)
}
