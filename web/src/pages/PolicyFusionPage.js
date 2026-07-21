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

import React, {useEffect, useMemo, useState} from "react";
import {Alert, Button, Card, Col, Empty, Row, Select, Space, Spin, Table, Tag, Tooltip, Typography} from "antd";
import {ArrowRightOutlined, EditOutlined, IdcardOutlined, MergeCellsOutlined, PlusOutlined} from "@ant-design/icons";
import {useHistory} from "react-router-dom";
import {getPolicySets} from "../backend/api";
import {FUSION_STRATEGIES, runFusion} from "../utils/policyFusion";
import {DecisionTag, PolicyEditor, editorStyle} from "./PolicyEditor";
import {EmployeeUnavailable, useEmployeePolicySet} from "./employeePolicySet";
import {AgentIcon, countPolicyRules} from "./policySetUtil";

const {Title, Text, Paragraph} = Typography;

// An intercepted call is never only a person's or only an agent's: it is this
// person, working through that agent. Policy fusion reconciles the two policy
// sets that each speak for one half of it - the digital employee's, stored on
// the Casdoor user, and the Policy Hub set guarding the agent - into a single
// verdict, and shows the reconciliation rather than just its result.

// StatCard is one stage of the fusion pipeline: which set it is, and how its
// verdicts came out over the calls being compared.
function StatCard({icon, title, subtitle, rows, decisionOf, highlight = false}) {
  const evaluated = rows.length;
  const allowed = rows.filter((row) => decisionOf(row).allowed).length;

  return (
    <Card
      size="small"
      style={{
        height: "100%",
        borderColor: highlight ? "#1677ff" : undefined,
        background: highlight ? "#f0f7ff" : undefined,
      }}
    >
      <Space align="start" size={10}>
        <div style={{fontSize: 20, lineHeight: "24px", color: highlight ? "#1677ff" : "#8c8c8c"}}>{icon}</div>
        <div style={{minWidth: 0}}>
          <Text strong style={{display: "block"}} ellipsis>{title}</Text>
          <Text type="secondary" style={{fontSize: 12}} ellipsis>{subtitle}</Text>
          <div style={{marginTop: 6}}>
            <Tag color="green" style={{marginInlineEnd: 4}}>{allowed} allow</Tag>
            <Tag color="red" style={{marginInlineEnd: 0}}>{evaluated - allowed} deny</Tag>
          </div>
        </div>
      </Space>
    </Card>
  );
}

function FusionArrow({symbol}) {
  return (
    <div style={{display: "flex", alignItems: "center", justifyContent: "center", height: "100%", minHeight: 40, color: "#bfbfbf", fontSize: 18}}>
      {symbol}
    </div>
  );
}

// The comparison table is the fusion itself, made visible: one row per call,
// the two sets' verdicts beside each other and the fused one last. A row where
// the two disagree is tinted, because those are the calls fusion decides.
function FusionTable({rows}) {
  const columns = [
    {
      title: "Destination",
      dataIndex: "obj",
      key: "obj",
      render: (value) => <Text style={editorStyle}>{value}</Text>,
    },
    {
      title: "Intent",
      dataIndex: "act",
      key: "act",
      width: 130,
      render: (value) => <Text style={editorStyle}>{value}</Text>,
    },
    {
      title: "Employee",
      key: "employee",
      width: 110,
      render: (_, row) => <DecisionTag row={row.employee} />,
    },
    {
      title: "Agent",
      key: "agent",
      width: 110,
      render: (_, row) => <DecisionTag row={row.agent} />,
    },
    {
      title: "Fused",
      key: "fused",
      width: 110,
      render: (_, row) => <DecisionTag row={row.fused} />,
    },
    {
      title: "Deciding rule",
      key: "reason",
      render: (_, row) => {
        if (row.fused.error) {
          return <Text type="danger" style={editorStyle}>{row.fused.error}</Text>;
        }
        if (row.fused.reason && row.fused.reason.length > 0) {
          return <Text style={editorStyle}>p, {row.fused.reason.join(", ")}</Text>;
        }
        return <Text type="secondary" style={editorStyle}>no rule matched</Text>;
      },
    },
  ];

  return (
    <Table
      rowKey={(row) => `${row.obj} ${row.act}`}
      dataSource={rows}
      columns={columns}
      pagination={false}
      size="small"
      rowClassName={(row) => (row.changed ? "aiguard-fusion-row-changed" : "")}
    />
  );
}

export default function PolicyFusionPage({account}) {
  const history = useHistory();
  const {policySet, loadError, reload} = useEmployeePolicySet();

  const [agentSets, setAgentSets] = useState([]);
  const [selectedName, setSelectedName] = useState("");
  const [strategy, setStrategy] = useState(FUSION_STRATEGIES[0].value);
  const [fusion, setFusion] = useState(null);
  const [fusionError, setFusionError] = useState(null);

  useEffect(() => {
    // The hub's sets arrive complete, model and policy included, so fusing with
    // one needs no second round trip when the operator picks it.
    getPolicySets().then(setAgentSets).catch(() => setAgentSets([]));
  }, []);

  // An enabled set is the one actually guarding this machine, so those are what
  // the picker offers first; the rest stay reachable because fusing is also how
  // you decide whether a set is worth enabling.
  const {options, defaultName} = useMemo(() => {
    const toOption = (set) => ({
      value: set.name,
      label: `${set.displayName}${set.agent ? ` · ${set.agent}` : ""}`,
    });
    const enabled = agentSets.filter((set) => set.enabled);
    const rest = agentSets.filter((set) => !set.enabled);

    const groups = [];
    if (enabled.length > 0) {
      groups.push({label: "Enabled on this machine", options: enabled.map(toOption)});
    }
    if (rest.length > 0) {
      groups.push({label: "Other policy sets", options: rest.map(toOption)});
    }
    return {options: groups, defaultName: (enabled[0] || rest[0] || {}).name || ""};
  }, [agentSets]);

  useEffect(() => {
    if (!selectedName && defaultName) {
      setSelectedName(defaultName);
    }
  }, [defaultName, selectedName]);

  const agentSet = useMemo(
    () => agentSets.find((set) => set.name === selectedName) || null,
    [agentSets, selectedName]
  );

  useEffect(() => {
    if (!policySet || !agentSet) {
      setFusion(null);
      return;
    }

    runFusion(policySet, agentSet, strategy)
      .then((result) => {
        setFusion(result);
        setFusionError(null);
      })
      .catch((err) => {
        setFusion(null);
        setFusionError(err.message);
      });
  }, [policySet, agentSet, strategy]);

  if (loadError) {
    return <EmployeeUnavailable error={loadError} account={account} onRetry={reload} />;
  }

  if (!policySet) {
    return <div style={{textAlign: "center", padding: "80px 0"}}><Spin size="large" /></div>;
  }

  const strategyInfo = FUSION_STRATEGIES.find((item) => item.value === strategy);

  return (
    <div>
      <style>{".aiguard-fusion-row-changed > td { background: #fffbe6; }"}</style>

      <div style={{display: "flex", alignItems: "flex-start", gap: 12}}>
        <div style={{flex: 1}}>
          <Title level={3} style={{margin: 0}}>
            <Space><MergeCellsOutlined />Policy Fusion</Space>
          </Title>
          <Paragraph type="secondary" style={{marginTop: 8, marginBottom: 0, maxWidth: 900}}>
            An intercepted call is never only a person&apos;s or only an agent&apos;s: it is <strong>{policySet.displayName}</strong>,
            working through an agent. Pick an agent&apos;s policy set to see the two reconciled - every call decided by your
            digital employee&apos;s set, by the agent&apos;s set, and by the two fused together.
          </Paragraph>
        </div>
        <Button icon={<EditOutlined />} onClick={() => history.push("/digital-employee")}>
          Edit my policy set
        </Button>
      </div>

      <Space wrap size={12} style={{margin: "20px 0 16px"}}>
        <Select
          style={{minWidth: 320}}
          value={selectedName || undefined}
          placeholder="Pick an agent's policy set to fuse with"
          options={options}
          onChange={setSelectedName}
        />
        <Tooltip title={strategyInfo && strategyInfo.description}>
          <Select
            style={{minWidth: 260}}
            value={strategy}
            options={FUSION_STRATEGIES.map((item) => ({value: item.value, label: item.label}))}
            onChange={setStrategy}
          />
        </Tooltip>
      </Space>

      {!policySet.stored &&
        <Alert
          type="info"
          showIcon
          style={{marginBottom: 16}}
          message="You are fusing with the starter policy set"
          description="You have never saved a digital employee policy set, so these are the starter rules written about you. Edit and save them on the Digital Employee page to fuse with your own."
        />
      }

      {agentSets.length > 0 && agentSets.every((set) => !set.enabled) &&
        <Alert
          type="info"
          showIcon
          style={{marginBottom: 16}}
          message="No policy set is enabled on this machine yet"
          description="Fusion works against any set, so you can preview one here first. Enable it on the Policy Hub page to have AIGuard enforce it for real."
        />
      }

      {fusionError && <Alert type="error" showIcon message={fusionError} style={{marginBottom: 16}} />}

      {!agentSet
        ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Pick a policy set to fuse with" />
        : !fusion
          ? <div style={{textAlign: "center", padding: "40px 0"}}><Spin /></div>
          : (
            <div>
              {fusion.fused.warnings.map((warning) => (
                <Alert key={warning} type="warning" showIcon message={warning} style={{marginBottom: 16}} />
              ))}

              <Row gutter={[8, 8]} align="stretch">
                <Col xs={24} md={7}>
                  <StatCard
                    icon={<IdcardOutlined />}
                    title={policySet.displayName}
                    subtitle={`digital employee · sub = ${fusion.fused.employeeSubject}`}
                    rows={fusion.rows}
                    decisionOf={(row) => row.employee}
                  />
                </Col>
                <Col xs={24} md={1}><FusionArrow symbol={<PlusOutlined />} /></Col>
                <Col xs={24} md={7}>
                  <StatCard
                    icon={<AgentIcon agent={agentSet.agent} size={20} fallback={agentSet.icon} />}
                    title={agentSet.displayName}
                    subtitle={`policy set · sub = ${fusion.fused.agentSubject}`}
                    rows={fusion.rows}
                    decisionOf={(row) => row.agent}
                  />
                </Col>
                <Col xs={24} md={1}><FusionArrow symbol={<ArrowRightOutlined />} /></Col>
                <Col xs={24} md={8}>
                  <StatCard
                    icon={<MergeCellsOutlined />}
                    title="Fused policy set"
                    subtitle={`${strategyInfo.label} · sub = ${fusion.fused.subject}`}
                    rows={fusion.rows}
                    decisionOf={(row) => row.fused}
                    highlight
                  />
                </Col>
              </Row>

              <Card
                size="small"
                title="Fused decisions"
                style={{marginTop: 16}}
                extra={
                  <Text type="secondary" style={{fontSize: 12}}>
                    {fusion.rows.filter((row) => row.changed).length} of {fusion.rows.length} calls the two sets decide differently
                  </Text>
                }
              >
                <FusionTable rows={fusion.rows} />
              </Card>

              <Row gutter={[16, 16]} style={{marginTop: 16}}>
                <Col xs={24} lg={12}>
                  <PolicyEditor
                    title="Fused policy"
                    extra={<Text type="secondary" style={{fontSize: 12}}>{countPolicyRules(fusion.fused.policy)} rules, generated</Text>}
                    value={fusion.fused.policy}
                    rows={18}
                    language="csv"
                    readOnly
                  />
                </Col>
                <Col xs={24} lg={12}>
                  <PolicyEditor
                    title="Fused requests"
                    extra={<Text type="secondary" style={{fontSize: 12}}>every call either set gives an example of</Text>}
                    value={fusion.fused.request}
                    rows={18}
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
