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

import React, {useEffect, useState} from "react";
import {Typography, Table, Button, Space, message, Select} from "antd";
import {getPolicy, updatePolicy} from "../backend/api";

const {Title, Text} = Typography;

const ALL_RECOGNIZERS = ["mcp", "payment-example"];
const ACTIONS = ["allow", "deny", "step-up", "pdp"];

export default function PolicyPage() {
  const [policy, setPolicy] = useState(null);

  const load = () => {
    getPolicy()
      .then(setPolicy)
      .catch((err) => message.error(err.message));
  };

  useEffect(load, []);

  if (!policy) {
    return <Text>Loading...</Text>;
  }

  const save = (next) => {
    updatePolicy(next)
      .then((saved) => {
        setPolicy(saved);
        message.success("Policy saved");
      })
      .catch((err) => message.error(err.message));
  };

  const updateRuleAction = (index, action) => {
    const rules = [...policy.rules];
    rules[index] = {...rules[index], action};
    setPolicy({...policy, rules});
  };

  const columns = [
    {title: "Rule ID", dataIndex: "id", key: "id"},
    {title: "Category", dataIndex: "category", key: "category"},
    {title: "Min Amount", dataIndex: "minAmount", key: "minAmount", render: (v) => v || "-"},
    {
      title: "Action",
      key: "action",
      render: (_, record, index) => (
        <Select
          value={record.action}
          style={{width: 120}}
          onChange={(v) => updateRuleAction(index, v)}
          options={ACTIONS.map((a) => ({label: a, value: a}))}
        />
      ),
    },
  ];

  return (
    <div>
      <Title level={3}>Policy</Title>

      <Title level={5}>Enabled Recognizers</Title>
      <Select
        mode="multiple"
        style={{width: 400, marginBottom: 24}}
        value={policy.enabledRecognizers || []}
        options={ALL_RECOGNIZERS.map((r) => ({label: r, value: r}))}
        onChange={(v) => setPolicy({...policy, enabledRecognizers: v})}
      />

      <Title level={5}>Destination Allowlist</Title>
      <Select
        mode="tags"
        style={{width: 400, marginBottom: 24}}
        value={policy.destinationAllowlist || []}
        onChange={(v) => setPolicy({...policy, destinationAllowlist: v})}
        placeholder="Hosts that always pass through, e.g. api.trusted.com"
      />

      <Title level={5}>Rules (evaluated top to bottom)</Title>
      <Table rowKey="id" dataSource={policy.rules || []} columns={columns} pagination={false} size="small" />

      <div style={{marginTop: 24}}>
        <Space>
          <Text>Default action for recognized traffic matching no rule:</Text>
          <Select
            value={policy.defaultAction}
            style={{width: 120}}
            options={ACTIONS.map((a) => ({label: a, value: a}))}
            onChange={(v) => setPolicy({...policy, defaultAction: v})}
          />
        </Space>
      </div>

      <Button type="primary" style={{marginTop: 24}} onClick={() => save(policy)}>
        Save Policy
      </Button>
    </div>
  );
}
