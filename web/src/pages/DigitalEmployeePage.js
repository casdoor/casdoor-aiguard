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

import React, {useCallback, useEffect, useState} from "react";
import {Alert, Avatar, Button, Card, Col, Row, Space, Spin, Tag, Tooltip, Typography, message} from "antd";
import {IdcardOutlined, MergeCellsOutlined, PlayCircleOutlined, ReloadOutlined, SaveOutlined} from "@ant-design/icons";
import {useHistory} from "react-router-dom";
import {updateEmployeePolicySet} from "../backend/api";
import {enforce} from "../utils/casbin";
import {PolicyEditor, ResultTable, editorStyle} from "./PolicyEditor";
import {EmployeeUnavailable, useEmployeePolicySet} from "./employeePolicySet";
import {countPolicyRules} from "./policySetUtil";

const {Title, Text, Paragraph} = Typography;

// A digital employee is the signed-in person seen as a Casbin subject: the same
// model/policy/requests a Policy Hub set is made of, but written about what
// this human is entitled to rather than what one agent may do. It is stored on
// their Casdoor user, so it follows them to any machine aiguard guards.
//
// This page is where that set is written. What it means together with an
// agent's set is the Policy Fusion page's job.

export default function DigitalEmployeePage({account}) {
  const history = useHistory();
  const {policySet, loadError, reload, setPolicySet} = useEmployeePolicySet();

  const [saving, setSaving] = useState(false);
  const [model, setModel] = useState("");
  const [policy, setPolicy] = useState("");
  const [request, setRequest] = useState("");

  const [results, setResults] = useState([]);
  const [error, setError] = useState(null);
  const [running, setRunning] = useState(false);

  const reset = useCallback((data) => {
    setModel(data.model || "");
    setPolicy(data.policy || "");
    setRequest(data.request || "");
  }, []);

  useEffect(() => {
    if (policySet) {
      reset(policySet);
    }
  }, [policySet, reset]);

  // Re-enforce shortly after typing stops, so editing the model, the policy or
  // the requests updates the decisions without pressing anything.
  useEffect(() => {
    if (!model) {
      return undefined;
    }

    const timer = setTimeout(() => {
      setRunning(true);
      enforce({model, policy, request})
        .then((rows) => {
          setResults(rows);
          setError(null);
        })
        .catch((err) => {
          setResults([]);
          setError(err.message);
        })
        .finally(() => setRunning(false));
    }, 300);

    return () => clearTimeout(timer);
  }, [model, policy, request]);

  const save = () => {
    setSaving(true);
    updateEmployeePolicySet({model, policy, request})
      .then((data) => {
        setPolicySet(data);
        message.success("Your digital employee's policy set was saved to Casdoor");
      })
      .catch((err) => message.error(err.message))
      .finally(() => setSaving(false));
  };

  if (loadError) {
    return <EmployeeUnavailable error={loadError} account={account} onRetry={reload} />;
  }

  if (!policySet) {
    return <div style={{textAlign: "center", padding: "80px 0"}}><Spin size="large" /></div>;
  }

  const dirty = model !== policySet.model || policy !== policySet.policy || request !== policySet.request;
  const allowed = results.filter((row) => row.allowed).length;
  const evaluated = results.filter((row) => !row.skipped).length;

  return (
    <div>
      <div style={{display: "flex", alignItems: "flex-start", gap: 14}}>
        <Avatar size={44} src={account && account.avatar} icon={<IdcardOutlined />} />
        <div style={{flex: 1}}>
          <Space align="center" wrap>
            <Title level={3} style={{margin: 0}}>{policySet.displayName}</Title>
            <Tag color="purple">digital employee</Tag>
            {policySet.stored
              ? <Tag color="blue">saved on Casdoor</Tag>
              : <Tooltip title="You have never saved a policy set, so these are starter rules written about you. Edit them and press Save."><Tag>starter template</Tag></Tooltip>
            }
          </Space>
          <Paragraph type="secondary" style={{marginTop: 8, marginBottom: 8, maxWidth: 900}}>
            Your own policy set: what you are entitled to do through <em>any</em> AI agent, as a Casbin model, policy and
            example requests. It is stored in your Casdoor user&apos;s properties, so it follows you to every machine AIGuard
            guards. Fuse it with an agent&apos;s policy set on the Policy Fusion page to see what the two allow together.
          </Paragraph>
          <Space wrap size={4}>
            <Tag color="geekblue" style={{margin: 0, fontFamily: editorStyle.fontFamily}}>sub = {policySet.subject}</Tag>
            <Tag style={{margin: 0}}>{policySet.owner}</Tag>
          </Space>
        </div>
        <Space>
          <Button icon={<MergeCellsOutlined />} onClick={() => history.push("/policy-fusion")}>Fuse with an agent</Button>
          <Button icon={<ReloadOutlined />} disabled={!dirty} onClick={() => reset(policySet)}>Reset changes</Button>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} disabled={!dirty} onClick={save}>Save</Button>
        </Space>
      </div>

      <Row gutter={[16, 16]} style={{marginTop: 20}}>
        <Col xs={24} lg={12}>
          <PolicyEditor title="Casbin model" value={model} rows={18} onChange={setModel} language="conf" />
        </Col>
        <Col xs={24} lg={12}>
          <Row gutter={[16, 16]}>
            <Col span={24}>
              <PolicyEditor
                title="Policy"
                extra={<Text type="secondary" style={{fontSize: 12}}>{countPolicyRules(policy)} rules</Text>}
                value={policy}
                rows={9}
                onChange={setPolicy}
                language="csv"
              />
            </Col>
            <Col span={24}>
              <PolicyEditor
                title="Requests"
                extra={<Text type="secondary" style={{fontSize: 12}}>one per line</Text>}
                value={request}
                rows={7}
                onChange={setRequest}
                language="csv"
              />
            </Col>
          </Row>
        </Col>
      </Row>

      <Card
        size="small"
        title="Enforcement result"
        style={{marginTop: 16}}
        extra={
          <Space>
            {running ? <Spin size="small" /> : <PlayCircleOutlined style={{color: "#52c41a"}} />}
            <Text type="secondary" style={{fontSize: 12}}>{allowed} of {evaluated} allowed</Text>
          </Space>
        }
      >
        {error ? <Alert type="error" message={error} /> : <ResultTable results={results} />}
      </Card>
    </div>
  );
}
