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

import React, {useEffect, useMemo, useRef, useState} from "react";
import {Alert, Card, Col, Row, Statistic, Table, Tag, Typography} from "antd";
import {ApiOutlined, RobotOutlined, SafetyCertificateOutlined, StopOutlined} from "@ant-design/icons";
import {Link} from "react-router-dom";
import * as echarts from "echarts";
import {getAgents, getEvents, getPolicySets, getRecords} from "../backend/api";

const {Title, Text} = Typography;

const DAY_MS = 24 * 60 * 60 * 1000;
const HOUR_MS = 60 * 60 * 1000;

// The console's theme is near-monochrome, so the charts stay in the same
// neutral family and spend the one loud color (red) on blocks only.
const COLOR_ALLOWED = "#525252";
const COLOR_BLOCKED = "#ef4444";
const COLOR_AXIS = "#a3a3a3";
const COLOR_SPLIT = "#f0f0f0";

// Categorical palette for the breakdown charts, mid-neutral through slate.
const CHART_COLORS = ["#262626", "#404040", "#525252", "#737373", "#8b8b8b", "#a3a3a3", "#b8b8b8"];

const CHART_HEIGHT = 300;

// Reusable ECharts container that handles initialization, option updates, and
// resize cleanup. Mirrors the widget in Casdoor's own dashboard.
const EchartsWidget = React.memo(({option, style}) => {
  const containerRef = useRef(null);
  const chartRef = useRef(null);

  useEffect(() => {
    if (!containerRef.current) {
      return;
    }
    const chart = echarts.init(containerRef.current);
    chartRef.current = chart;

    const observer = new ResizeObserver(() => chart.resize());
    observer.observe(containerRef.current);

    return () => {
      observer.disconnect();
      chart.dispose();
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (chartRef.current && option) {
      chartRef.current.setOption(option, {notMerge: true});
    }
  }, [option]);

  return <div ref={containerRef} style={style} />;
});

EchartsWidget.displayName = "EchartsWidget";

// countBy tallies rows by a key, dropping the ones with no key, and returns the
// busiest first. Every breakdown chart on this page is the same shape.
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

// A chart with no data draws an empty grid that reads as a bug, so charts fall
// back to this instead.
function EmptyChart({text}) {
  return (
    <div style={{height: CHART_HEIGHT, display: "flex", alignItems: "center", justifyContent: "center", color: "#a3a3a3", textAlign: "center", padding: "0 24px"}}>
      {text}
    </div>
  );
}

// buildHourlyOption plots the last 24 hours of records as stacked bars, so the
// mix of allowed and blocked traffic and its rhythm read together.
function buildHourlyOption(records, since) {
  const startHour = new Date(since);
  startHour.setMinutes(0, 0, 0);
  const labels = [];
  const allowed = [];
  const blocked = [];
  for (let i = 0; i < 24; i++) {
    const bucket = new Date(startHour.getTime() + i * HOUR_MS);
    labels.push(`${String(bucket.getHours()).padStart(2, "0")}:00`);
    allowed.push(0);
    blocked.push(0);
  }

  records.forEach((record) => {
    const index = Math.floor((new Date(record.createdTime).getTime() - startHour.getTime()) / HOUR_MS);
    if (index >= 0 && index < 24) {
      if (record.isAllowed) {
        allowed[index] += 1;
      } else {
        blocked[index] += 1;
      }
    }
  });

  return {
    tooltip: {trigger: "axis", axisPointer: {type: "shadow"}},
    legend: {top: 0, right: 0, itemWidth: 10, itemHeight: 10, textStyle: {color: "#525252"}},
    grid: {left: 8, right: 8, bottom: 0, top: 40, containLabel: true},
    xAxis: {
      type: "category",
      data: labels,
      axisLine: {lineStyle: {color: COLOR_SPLIT}},
      axisTick: {show: false},
      axisLabel: {color: COLOR_AXIS, interval: 2},
    },
    yAxis: {
      type: "value",
      minInterval: 1,
      axisLabel: {color: COLOR_AXIS},
      splitLine: {lineStyle: {color: COLOR_SPLIT}},
    },
    series: [
      {
        name: "Allowed",
        type: "bar",
        stack: "total",
        barMaxWidth: 18,
        itemStyle: {color: COLOR_ALLOWED, borderRadius: [0, 0, 2, 2]},
        data: allowed,
      },
      {
        name: "Blocked",
        type: "bar",
        stack: "total",
        barMaxWidth: 18,
        itemStyle: {color: COLOR_BLOCKED, borderRadius: [2, 2, 0, 0]},
        data: blocked,
      },
    ],
  };
}

// buildDonutOption shows the allow/block split, with the block count kept in
// the middle because that is the number an operator is looking for.
function buildDonutOption(allowedCount, blockedCount) {
  const total = allowedCount + blockedCount;
  return {
    tooltip: {trigger: "item", formatter: "{b}: {c} ({d}%)"},
    legend: {bottom: 0, itemWidth: 10, itemHeight: 10, textStyle: {color: "#525252"}},
    title: {
      text: String(blockedCount),
      subtext: `blocked of ${total}`,
      left: "center",
      top: "38%",
      textStyle: {fontSize: 28, fontWeight: 600, color: blockedCount ? COLOR_BLOCKED : "#262626"},
      subtextStyle: {fontSize: 12, color: "#a3a3a3"},
    },
    series: [{
      type: "pie",
      radius: ["62%", "82%"],
      center: ["50%", "46%"],
      avoidLabelOverlap: true,
      itemStyle: {borderColor: "#fff", borderWidth: 2},
      label: {show: false},
      emphasis: {label: {show: false}, itemStyle: {shadowBlur: 10, shadowColor: "rgba(0,0,0,0.2)"}},
      data: [
        {name: "Allowed", value: allowedCount, itemStyle: {color: COLOR_ALLOWED}},
        {name: "Blocked", value: blockedCount, itemStyle: {color: COLOR_BLOCKED}},
      ],
    }],
  };
}

// buildRankOption draws a ranked breakdown as horizontal bars. ECharts stacks a
// category axis bottom-up, so the list is reversed to put the top entry first.
function buildRankOption(items, color) {
  const top = items.slice(0, 8).reverse();
  return {
    tooltip: {trigger: "axis", axisPointer: {type: "shadow"}},
    grid: {left: 8, right: 32, bottom: 0, top: 8, containLabel: true},
    xAxis: {
      type: "value",
      minInterval: 1,
      axisLabel: {color: COLOR_AXIS},
      splitLine: {lineStyle: {color: COLOR_SPLIT}},
    },
    yAxis: {
      type: "category",
      data: top.map((item) => item.key),
      axisLine: {show: false},
      axisTick: {show: false},
      axisLabel: {color: "#525252", width: 160, overflow: "truncate"},
    },
    series: [{
      type: "bar",
      barMaxWidth: 18,
      itemStyle: {color: color || CHART_COLORS[1], borderRadius: [0, 4, 4, 0]},
      label: {show: true, position: "right", color: COLOR_AXIS},
      data: top.map((item) => item.count),
    }],
  };
}

// buildDestinationOption ranks the hosts intercepted traffic went to, colored by
// the proxy's decision so a denied destination stands out in the same bar.
function buildDestinationOption(events) {
  const hosts = new Map();
  events.forEach((event) => {
    const host = (event.destination || "").split(":")[0];
    if (!host) {
      return;
    }
    const entry = hosts.get(host) || {allowed: 0, blocked: 0};
    if (event.decision === "deny") {
      entry.blocked += 1;
    } else {
      entry.allowed += 1;
    }
    hosts.set(host, entry);
  });

  const top = Array.from(hosts, ([host, counts]) => ({host, ...counts}))
    .sort((a, b) => (b.allowed + b.blocked) - (a.allowed + a.blocked))
    .slice(0, 8)
    .reverse();

  return {
    tooltip: {trigger: "axis", axisPointer: {type: "shadow"}},
    legend: {top: 0, right: 0, itemWidth: 10, itemHeight: 10, textStyle: {color: "#525252"}},
    grid: {left: 8, right: 32, bottom: 0, top: 40, containLabel: true},
    xAxis: {
      type: "value",
      minInterval: 1,
      axisLabel: {color: COLOR_AXIS},
      splitLine: {lineStyle: {color: COLOR_SPLIT}},
    },
    yAxis: {
      type: "category",
      data: top.map((item) => item.host),
      axisLine: {show: false},
      axisTick: {show: false},
      axisLabel: {color: "#525252", width: 180, overflow: "truncate"},
    },
    series: [
      {
        name: "Allowed",
        type: "bar",
        stack: "total",
        barMaxWidth: 18,
        itemStyle: {color: COLOR_ALLOWED},
        data: top.map((item) => item.allowed),
      },
      {
        name: "Blocked",
        type: "bar",
        stack: "total",
        barMaxWidth: 18,
        itemStyle: {color: COLOR_BLOCKED, borderRadius: [0, 4, 4, 0]},
        data: top.map((item) => item.blocked),
      },
    ],
  };
}

export default function DashboardPage() {
  const [records, setRecords] = useState([]);
  const [agents, setAgents] = useState([]);
  const [policySets, setPolicySets] = useState([]);
  const [events, setEvents] = useState([]);
  const [error, setError] = useState(null);

  useEffect(() => {
    const load = () => {
      Promise.all([
        getRecords("", 200),
        getAgents(),
        getPolicySets(),
        // The traffic itself is on the Interception page; here it only feeds the
        // destination breakdown and the intercepted-request count.
        getEvents(200),
      ])
        .then(([recordData, agentData, policySetData, eventData]) => {
          setRecords(recordData || []);
          setAgents(agentData || []);
          setPolicySets(policySetData || []);
          setEvents(eventData || []);
          setError(null);
        })
        .catch((err) => setError(err.message));
    };
    load();
    const interval = setInterval(load, 5000);
    return () => clearInterval(interval);
  }, []);

  // The window is recomputed per render so the charts keep sliding with the
  // clock even between polls.
  const since = Date.now() - DAY_MS;
  const recent = records.filter((record) => new Date(record.createdTime).getTime() >= since);
  const blocked = recent.filter((record) => !record.isAllowed);

  const patchedCount = agents.filter((agent) => agent.patched).length;
  const enabledCount = policySets.filter((policySet) => policySet.enabled).length;

  const blockedBySet = countBy(blocked, (record) => record.policySet);
  const byAgent = countBy(recent, (record) => record.agent);

  const hourlyOption = useMemo(() => buildHourlyOption(recent, since), [records]);
  const donutOption = useMemo(() => buildDonutOption(recent.length - blocked.length, blocked.length), [records]);
  const blockedBySetOption = useMemo(() => buildRankOption(blockedBySet, COLOR_BLOCKED), [records]);
  const byAgentOption = useMemo(() => buildRankOption(byAgent, CHART_COLORS[1]), [records]);
  const destinationOption = useMemo(() => buildDestinationOption(events), [events]);

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
            <Statistic title="Agents patched" value={patchedCount} suffix={`/ ${agents.length}`} prefix={<RobotOutlined style={{color: "#737373"}} />} />
            <Link to="/agents">Manage agents</Link>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic title="Policy sets enabled" value={enabledCount} suffix={`/ ${policySets.length}`} prefix={<SafetyCertificateOutlined style={{color: "#737373"}} />} />
            <Link to="/policyhub">Policy Hub</Link>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic title="Records (24h)" value={recent.length} prefix={<ApiOutlined style={{color: "#737373"}} />} />
            <Link to="/records">All records</Link>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic
              title="Blocked (24h)"
              value={blocked.length}
              prefix={<StopOutlined style={{color: blocked.length ? COLOR_BLOCKED : "#737373"}} />}
              styles={{content: {color: blocked.length ? COLOR_BLOCKED : undefined}}}
            />
            {/* Proxy traffic is a separate signal from the hook-reported records
                above: zero here means interception is not carrying traffic yet. */}
            <Link to="/intercept">{events.length > 0 ? `${events.length} intercepted requests` : "Interception idle"}</Link>
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{marginTop: 16}}>
        <Col xs={24} xl={16}>
          <Card size="small" title="Records per hour (24h)">
            {recent.length > 0
              ? <EchartsWidget option={hourlyOption} style={{height: CHART_HEIGHT}} />
              : <EmptyChart text="No agent activity - patch an agent to start collecting records" />}
          </Card>
        </Col>
        <Col xs={24} xl={8}>
          <Card size="small" title="Allowed vs blocked (24h)">
            {recent.length > 0
              ? <EchartsWidget option={donutOption} style={{height: CHART_HEIGHT}} />
              : <EmptyChart text="Nothing evaluated in the last 24 hours" />}
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{marginTop: 16}}>
        <Col xs={24} xl={12}>
          <Card size="small" title="Blocks by policy set (24h)" extra={<Link to="/policyhub">Policy Hub</Link>}>
            {blockedBySet.length > 0
              ? <EchartsWidget option={blockedBySetOption} style={{height: CHART_HEIGHT}} />
              : <EmptyChart text="Nothing blocked in the last 24 hours" />}
          </Card>
        </Col>
        <Col xs={24} xl={12}>
          <Card size="small" title="Activity by agent (24h)" extra={<Link to="/records">All records</Link>}>
            {byAgent.length > 0
              ? <EchartsWidget option={byAgentOption} style={{height: CHART_HEIGHT}} />
              : <EmptyChart text="No agent activity yet" />}
          </Card>
        </Col>
      </Row>

      <Card size="small" title="Top intercepted destinations" style={{marginTop: 16}} extra={<Link to="/intercept">Event stream</Link>}>
        {events.length > 0
          ? <EchartsWidget option={destinationOption} style={{height: CHART_HEIGHT}} />
          : <EmptyChart text="No intercepted traffic - enable interception to see egress destinations" />}
      </Card>

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
