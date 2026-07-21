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
import {Alert, Button, Card, Col, Empty, Popconfirm, Row, Space, Spin, Statistic, Table, Tag, Tooltip, Typography, message} from "antd";
import {BulbOutlined, DeleteOutlined, FileTextOutlined, MergeCellsOutlined, ReloadOutlined, RobotOutlined} from "@ant-design/icons";
import {Link, useHistory} from "react-router-dom";
import {deleteLearnedRule} from "../backend/api";
import {PolicyEditor, editorStyle} from "./PolicyEditor";
import {EmployeeUnavailable, useLearnedPolicySet} from "./employeePolicySet";
import {AgentIcon, countPolicyRules} from "./policySetUtil";

const {Title, Text, Paragraph} = Typography;

// The policy sets everywhere else in aiguard were written by somebody in
// advance: a guess about what an agent will need to do. This one was not. Every
// rule here came from a real intercepted call that aiguard decided wrongly and a
// person corrected on the Records page - so this set is the only one that grew
// out of what actually happened on this machine rather than out of a guess.
//
// The rules live on the Casdoor user, beside the digital employee's, which means
// a lesson learned here is a lesson learned everywhere that person signs in.

// RuleTable is the provenance view: not "here are some rules" but "here is the
// call, here is what aiguard said, here is what you said". A learned rule that
// cannot be traced back to the record it came from is a rule nobody will trust.
function RuleTable({rules, onForget, forgetting}) {
  const columns = [
    {
      title: "Destination",
      dataIndex: "object",
      key: "object",
      render: (value) => <Text style={editorStyle}>{value}</Text>,
    },
    {
      title: "Intent",
      dataIndex: "action",
      key: "action",
      width: 140,
      render: (value) => <Text style={editorStyle}>{value}</Text>,
    },
    {
      title: "Agent",
      dataIndex: "agent",
      key: "agent",
      width: 150,
      render: (value) => (value ? (
        <Tag color="blue" style={{display: "inline-flex", alignItems: "center", gap: 6}}>
          <AgentIcon agent={value} size={16} fallback={<RobotOutlined />} />
          {value}
        </Tag>
      ) : null),
    },
    {
      title: "AIGuard said",
      key: "was",
      width: 120,
      render: (_, rule) => (
        <Tooltip title={rule.reason || (rule.policySet ? `decided by the policy set "${rule.policySet}"` : "no enabled policy set objected")}>
          <Tag color={rule.wasAllowed ? "green" : "red"}>{rule.wasAllowed ? "allow" : "deny"}</Tag>
        </Tooltip>
      ),
    },
    {
      title: "You said",
      key: "effect",
      width: 120,
      render: (_, rule) => <Tag color={rule.effect === "deny" ? "red" : "green"}>{rule.effect}</Tag>,
    },
    {
      title: "Learned rule",
      key: "rule",
      render: (_, rule) => (
        <Text style={editorStyle} type="secondary">
          p, {rule.subject}, ^{rule.object}$, ^{rule.action}$, {rule.effect}
        </Text>
      ),
    },
    {
      title: "",
      key: "forget",
      width: 90,
      render: (_, rule) => (
        <Popconfirm
          title="Forget this rule?"
          description="The correction on the record it came from is withdrawn too, so the two never disagree."
          onConfirm={() => onForget(rule.id)}
          okText="Forget"
        >
          <Button size="small" type="text" danger icon={<DeleteOutlined />} loading={forgetting === rule.id}>Forget</Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <Table
      rowKey="id"
      dataSource={rules}
      columns={columns}
      pagination={rules.length > 20 ? {pageSize: 20} : false}
      size="small"
    />
  );
}

export default function SelfLearningPage({account}) {
  const history = useHistory();
  const {policySet, loadError, reload, setPolicySet} = useLearnedPolicySet();
  const [forgetting, setForgetting] = useState("");

  // Corrections are made on another page, so what is shown here goes stale
  // while it is open. Re-reading periodically keeps the two in step without
  // asking anyone to press anything.
  useEffect(() => {
    const interval = setInterval(reload, 5000);
    return () => clearInterval(interval);
  }, [reload]);

  if (loadError) {
    return <EmployeeUnavailable error={loadError} account={account} onRetry={reload} />;
  }

  if (!policySet) {
    return <div style={{textAlign: "center", padding: "80px 0"}}><Spin size="large" /></div>;
  }

  const rules = policySet.rules || [];
  const overturnedDenies = rules.filter((rule) => !rule.wasAllowed && rule.effect === "allow");
  const overturnedAllows = rules.filter((rule) => rule.wasAllowed && rule.effect === "deny");

  const forget = (id) => {
    setForgetting(id);
    deleteLearnedRule(id)
      .then((data) => {
        setPolicySet(data);
        message.success("The rule was forgotten and the correction behind it withdrawn");
      })
      .catch((err) => message.error(err.message))
      .finally(() => setForgetting(""));
  };

  return (
    <div>
      <div style={{display: "flex", alignItems: "flex-start", gap: 12}}>
        <div style={{flex: 1}}>
          <Space align="center" wrap>
            <Title level={3} style={{margin: 0}}>
              <Space><BulbOutlined />Self-Learning Policy</Space>
            </Title>
            <Tag color="gold">generated</Tag>
            <Tag color="blue">saved on Casdoor</Tag>
          </Space>
          <Paragraph type="secondary" style={{marginTop: 8, marginBottom: 8, maxWidth: 900}}>
            Every other policy set in AIGuard was written in advance - a guess about what an agent would need to do. This one
            grew out of what actually happened here: each rule below came from a real intercepted call that AIGuard decided
            wrongly and <strong>{policySet.displayName}</strong> corrected in the <Link to="/records">Records</Link> table&apos;s
            Feedback column. The rules are stored on your Casdoor user, so they follow you to every machine AIGuard guards, and
            they are the third input to <Link to="/policy-fusion">Policy Fusion</Link>.
          </Paragraph>
          <Space wrap size={4}>
            <Tag color="geekblue" style={{margin: 0, fontFamily: editorStyle.fontFamily}}>sub = {policySet.subject}</Tag>
            <Tag style={{margin: 0}}>{policySet.owner}</Tag>
          </Space>
        </div>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={reload}>Refresh</Button>
          <Button icon={<FileTextOutlined />} onClick={() => history.push("/records")}>Correct a record</Button>
          <Button type="primary" icon={<MergeCellsOutlined />} onClick={() => history.push("/policy-fusion")}>Fuse</Button>
        </Space>
      </div>

      {rules.length === 0
        ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            style={{padding: "60px 0"}}
            description={
              <div style={{maxWidth: 520, margin: "0 auto"}}>
                <Paragraph style={{marginBottom: 4}}>Nothing has been learned yet.</Paragraph>
                <Text type="secondary">
                  A policy set written in advance will get calls wrong - it will block something that was fine. When it does,
                  open the Records page, find the blocked row and set its Feedback column to &quot;should be allowed&quot;. The
                  rule that says so appears here.
                </Text>
              </div>
            }
          >
            <Button type="primary" icon={<FileTextOutlined />} onClick={() => history.push("/records")}>Go to Records</Button>
          </Empty>
        )
        : (
          <div>
            <Row gutter={[16, 16]} style={{marginTop: 20}}>
              <Col xs={12} md={8}>
                <Card size="small"><Statistic title="Rules learned" value={rules.length} prefix={<BulbOutlined />} /></Card>
              </Col>
              <Col xs={12} md={8}>
                <Card size="small">
                  <Statistic
                    title="Blocks overturned"
                    value={overturnedDenies.length}
                    valueStyle={{color: "#52c41a"}}
                  />
                </Card>
              </Col>
              <Col xs={12} md={8}>
                <Card size="small">
                  <Statistic
                    title="Allows tightened"
                    value={overturnedAllows.length}
                    valueStyle={{color: "#cf1322"}}
                  />
                </Card>
              </Col>
            </Row>

            <Alert
              type="info"
              showIcon
              style={{marginTop: 16}}
              message="A correction is about one call, not a category"
              description="Each rule is anchored to the exact destination and intent that was observed, so saying one blocked call was fine never quietly widens into a permission you did not grant. Generalizing a rule is a deliberate edit, made on your digital employee's policy set."
            />

            <Card
              size="small"
              title="What AIGuard learned"
              style={{marginTop: 16}}
              extra={<Text type="secondary" style={{fontSize: 12}}>one rule per corrected record</Text>}
            >
              <RuleTable rules={rules} onForget={forget} forgetting={forgetting} />
            </Card>

            <Row gutter={[16, 16]} style={{marginTop: 16}}>
              <Col xs={24} lg={12}>
                <PolicyEditor
                  title="Self-learned policy"
                  extra={<Text type="secondary" style={{fontSize: 12}}>{countPolicyRules(policySet.policy)} lines, generated</Text>}
                  value={policySet.policy}
                  rows={16}
                  language="csv"
                  readOnly
                />
              </Col>
              <Col xs={24} lg={12}>
                <PolicyEditor
                  title="The calls it was learned from"
                  extra={<Text type="secondary" style={{fontSize: 12}}>one per corrected record</Text>}
                  value={policySet.request}
                  rows={16}
                  language="csv"
                  readOnly
                />
              </Col>
            </Row>
          </div>
        )
      }
    </div>
  );
}
