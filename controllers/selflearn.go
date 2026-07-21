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

package controllers

import (
	"encoding/json"

	"github.com/casdoor/casdoor-aiguard/auth"
	"github.com/casdoor/casdoor-aiguard/object"
)

// Feedback on a record and the self-learned policy set are two views of one
// thing: the operator says a verdict was wrong, and that judgement becomes a
// Casbin rule. So the two live here together - correcting a record is what
// writes a learned rule, and the learned set is only ever read back.
//
// Like the digital employee, the rules are stored on the Casdoor user rather
// than on this host, which means both endpoints need a signed-in person. That
// is not incidental: a correction is somebody's judgement, and an anonymous one
// would be a rule nobody is accountable for.

// learnedPolicySetOf reads a user's learned rules and renders them as a policy
// set, using the same subject their digital employee's rules are written about
// so the two fuse without rewriting anything.
func learnedPolicySetOf(user *auth.User) *object.LearnedPolicySet {
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Name
	}
	return object.BuildLearnedPolicySet(
		user.Owner,
		user.Name,
		displayName,
		user.Name,
		object.ParseLearnedRules(user.Properties[object.LearnedRulesProperty]),
	)
}

// GetLearnedPolicySet
// @Title GetLearnedPolicySet
// @Description get the signed-in user's self-learned policy set - the Casbin rules
// @Description derived from the records they corrected, stored in their Casdoor
// @Description user's properties
// @router /learned-policy-set [get]
func (c *ApiController) GetLearnedPolicySet() {
	user := c.requireCasdoorUser()
	if user == nil {
		return
	}

	c.ResponseOk(learnedPolicySetOf(user))
}

// UpdateRecordFeedback is where self-learning actually happens: it writes the
// operator's correction onto the record *and* derives the rule that correction
// implies, in one step, so the Records page and the Self-Learning page can never
// disagree about what was taught.
//
// Withdrawing feedback (an empty verdict) unlearns the rule the same way.
//
// @Title UpdateRecordFeedback
// @Description correct the verdict on one record and update the self-learned policy set accordingly
// @router /records/feedback [post]
func (c *ApiController) UpdateRecordFeedback() {
	var body struct {
		Id       string `json:"id"`
		Feedback string `json:"feedback"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &body); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if body.Id == "" {
		c.ResponseError("the record id is required")
		return
	}

	user := c.requireCasdoorUser()
	if user == nil {
		return
	}

	record, err := object.SetRecordFeedback(body.Id, body.Feedback, user.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	// The rule is derived from the corrected record rather than from the request
	// body, so what is learned is always what the audit trail says happened.
	var rule *object.LearnedRule
	if record.Feedback != object.FeedbackNone {
		rule, err = object.LearnedRuleFromRecord(record, user.Name)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
	}

	rules := object.UpsertLearnedRule(
		object.ParseLearnedRules(user.Properties[object.LearnedRulesProperty]),
		record.Id,
		rule,
	)
	value, err := object.MarshalLearnedRules(rules)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	if user.Properties == nil {
		user.Properties = map[string]string{}
	}
	user.Properties[object.LearnedRulesProperty] = value

	if err := auth.UpdateUserProperties(user); err != nil {
		// The record already carries the correction, so say what did and did not
		// stick rather than pretending the whole thing failed.
		c.ResponseError("the record was corrected, but saving the learned rule to Casdoor failed: " + err.Error())
		return
	}

	c.ResponseOk(map[string]any{
		"record":    record,
		"policySet": learnedPolicySetOf(user),
	})
}

// DeleteLearnedRule drops one learned rule and the feedback it came from, so
// forgetting a lesson from the Self-Learning page leaves the record it was
// learned from uncorrected rather than silently disagreeing with it.
//
// @Title DeleteLearnedRule
// @Description forget one self-learned rule and withdraw the record feedback behind it
// @router /learned-policy-set/delete [post]
func (c *ApiController) DeleteLearnedRule() {
	var body struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &body); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if body.Id == "" {
		c.ResponseError("the rule id is required")
		return
	}

	user := c.requireCasdoorUser()
	if user == nil {
		return
	}

	rules := object.UpsertLearnedRule(
		object.ParseLearnedRules(user.Properties[object.LearnedRulesProperty]),
		body.Id,
		nil,
	)
	value, err := object.MarshalLearnedRules(rules)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	if user.Properties == nil {
		user.Properties = map[string]string{}
	}
	user.Properties[object.LearnedRulesProperty] = value

	if err := auth.UpdateUserProperties(user); err != nil {
		c.ResponseError(err.Error())
		return
	}

	// The record may well have aged out of the ring buffer long after its rule
	// was learned, which is fine: the rule is gone either way.
	_, _ = object.SetRecordFeedback(body.Id, object.FeedbackNone, user.Name)

	c.ResponseOk(learnedPolicySetOf(user))
}
