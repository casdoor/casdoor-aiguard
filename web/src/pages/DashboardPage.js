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
import {Alert, Card, Col, Empty, Progress, Row, Statistic, Table, Tag, Typography} from "antd";
import {Link} from "react-router-dom";
import {getAgents, getEvents, getPolicySets, getRecords} from "../backend/api";

const {Title, Text} = Typography;

const DAY_MS = 24 * 60 * 60 * 1000;

// countBy tallies rows by a key, dropping the ones with no key, and returns the
// busiest first. Both breakdown lists on this page are the same shape.
function countBy(rows, keyOf) {
  const counts = new Map();
  rows.forEach((row) => {
    const key = keyOf(row);
    if (key) {
      counts.set(key, (counts.get(key) || 0) + 1);
    }
  });
  return Array.from(counts, ([key, count]) => ({key, count})).sort((a, b) => b.count - a.count);
}

// BarList renders a ranked breakdown as bars scaled against the top entry, so
// the shape of the distribution reads at a glance without a chart library.
function BarList({items, color, renderLabel, emptyText}) {
  if (items.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />;
  }
  const top = items[0].count;
  return (
    <div>
      {items.map((item) => (
        <div key={item.key} style={{marginBottom: 12}}>
          <div style={{display: "flex", justifyContent: "space-between", gap: 8}}>
            <Text ellipsis style={{flex: 1}}>{renderLabel ? renderLabel(item.key) : item.key}</Text>
            <Text strong>{item.count}</Text>
          </div>
          <Progress percent={Math.round((item.count / top) * 100)} showInfo={false} size="small" strokeColor={color} />
        </div>
      ))}
    </div>
  );
}

export default function DashboardPage() {
  const [records, setRecords] = useState([]);
  const [agents, setAgents] = useState([]);
  const [policySets, setPolicySets] = useState([]);
  const [eventCount, setEventCount] = useState(0);
  const [error, setError] = useState(null);

  useEffect(() => {
    const load = () => {
      Promise.all([
        getRecords("", 200),
        getAgents(),
        getPolicySets(),
        // Only the volume matters here - the traffic itself is on the
        // Interception page, next to the settings that produce it.
        getEvents(200),
      ])
        .then(([recordData, agentData, policySetData, eventData]) => {
          setRecords(recordData || []);
          setAgents(agentData || []);
          setPolicySets(policySetData || []);
          setEventCount((eventData || []).length);
          setError(null);
        })
        .catch((err) => setError(err.message));
    };
    load();
    const interval = setInterval(load, 5000);
    return () => clearInterval(interval);
  }, []);

  const since = Date.now() - DAY_MS;
  const recent = records.filter((record) => new Date(record.createdTime).getTime() >= since);
  const blocked = recent.filter((record) => !record.isAllowed);

  const patchedCount = agents.filter((agent) => agent.patched).length;
  const enabledCount = policySets.filter((policySet) => policySet.enabled).length;

  const blockedBySet = countBy(blocked, (record) => record.policySet);
  const byAgent = countBy(recent, (record) => record.agent);

  const blockColumns = [
    {title: "Time", dataIndex: "createdTime", key: "createdTime", render: (v) => new Date(v).toLocaleTimeString()},
    {title: "Agent", dataIndex: "agent", key: "agent", render: (v) => <Tag color="blue">{v}</Tag>},
    {title: "Operation", key: "event", render: (_, record) => <Text code>{`${record.eventType}:${record.action}`}</Text>},
    {
      title: "Policy set",
      dataIndex: "policySet",
      key: "policySet",
      ellipsis: true,
      render: (value) => (value ? <Link to={`/policyhub/${encodeURIComponent(value)}`}>{value}</Link> : null),
    },
    {title: "Reason", dataIndex: "reason", key: "reason", ellipsis: true, render: (v) => <Text type="danger">{v}</Text>},
  ];

  return (
    <div>
      <Title level={3}>Dashboard</Title>
      {error && <Alert type="error" message={error} style={{marginBottom: 16}} />}

      <Row gutter={[16, 16]}>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic title="Agents patched" value={patchedCount} suffix={`/ ${agents.length}`} />
            <Link to="/agents">Manage agents</Link>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic title="Policy sets enabled" value={enabledCount} suffix={`/ ${policySets.length}`} />
            <Link to="/policyhub">Policy Hub</Link>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic title="Records (24h)" value={recent.length} />
            <Link to="/records">All records</Link>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic title="Blocked (24h)" value={blocked.length} styles={{content: {color: blocked.length ? "#cf1322" : undefined}}} />
            {/* Proxy traffic is a separate signal from the hook-reported records
                above: zero here means interception is not carrying traffic yet. */}
            <Link to="/intercept">{eventCount > 0 ? `${eventCount} intercepted requests` : "Interception idle"}</Link>
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{marginTop: 16}}>
        <Col xs={24} lg={12}>
          <Card size="small" title="Blocks by policy set (24h)">
            <BarList
              items={blockedBySet}
              color="#cf1322"
              renderLabel={(name) => <Link to={`/policyhub/${encodeURIComponent(name)}`}>{name}</Link>}
              emptyText="Nothing blocked in the last 24 hours"
            />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card size="small" title="Activity by agent (24h)">
            <BarList
              items={byAgent}
              color="#1677ff"
              renderLabel={(name) => <Link to={`/records?agent=${encodeURIComponent(name)}`}>{name}</Link>}
              emptyText="No agent activity - patch an agent to start collecting records"
            />
          </Card>
        </Col>
      </Row>

      <Card size="small" title="Recent blocks" style={{marginTop: 16}} extra={<Link to="/records">View all records</Link>}>
        <Table
          rowKey="id"
          dataSource={blocked.slice(0, 10)}
          columns={blockColumns}
          pagination={false}
          size="small"
          locale={{emptyText: "No blocks in the last 24 hours"}}
        />
      </Card>
    </div>
  );
}
