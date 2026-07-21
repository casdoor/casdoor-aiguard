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
import {Alert, Button, Card, Col, Empty, Input, Row, Select, Spin, Switch, Tag, Tooltip, Typography, message} from "antd";
import {ArrowRightOutlined, QuestionCircleOutlined, SafetyCertificateOutlined} from "@ant-design/icons";
import {useHistory} from "react-router-dom";
import {getPolicySets, setPolicySetEnabled} from "../backend/api";
import {AGENT_ORDER, AgentIcon, OS_FAMILIES, STRICTNESS_COLORS, STRICTNESS_ORDER, countPolicyRules, osFamily, sortByOrder} from "./policySetUtil";

const {Title, Text, Paragraph} = Typography;

// Every card is filtered by the same three facets the tags advertise: which
// agent it is written for, which OS it runs on, and how tight it is.
function uniqueValues(policySets, field) {
  const values = new Set();
  policySets.forEach((policySet) => policySet[field] && values.add(policySet[field]));
  return Array.from(values);
}

// A set names one OS, so selecting "Linux" has to match its distributions too.
function matchesOs(policySet, os) {
  return policySet.os === os || osFamily(policySet.os) === os;
}

function matches(policySet, {search, agent, os, strictness}) {
  if (agent && policySet.agent !== agent) {
    return false;
  }
  if (os && !matchesOs(policySet, os)) {
    return false;
  }
  if (strictness && policySet.strictness !== strictness) {
    return false;
  }
  if (!search) {
    return true;
  }

  const haystack = [
    policySet.name,
    policySet.displayName,
    policySet.description,
    policySet.agent,
    policySet.os,
    ...(policySet.tags || []),
  ].join(" ").toLowerCase();
  return haystack.includes(search.toLowerCase());
}

// The OS filter offers each family as a heading a reader can pick outright,
// with the distributions that actually have policy sets nested under it.
function osOptions(policySets) {
  const present = uniqueValues(policySets, "os");

  return [
    {value: "", label: "OS: All"},
    ...OS_FAMILIES.filter((family) => present.some((os) => family.members.includes(os))).map((family) => {
      const members = sortByOrder(present.filter((os) => family.members.includes(os)), family.members);
      if (members.length === 1 && members[0] === family.name) {
        return {value: family.name, label: family.name};
      }
      return {
        label: family.name,
        options: [
          {value: family.name, label: `${family.name} (all)`},
          ...members.map((os) => ({value: os, label: os})),
        ],
      };
    }),
  ];
}

// EnableControl is the switch that turns a set on for live interception. It sits
// inside the card, which navigates on click, so every interaction stops the
// click from bubbling. A set the host cannot enforce (agent not patched, wrong
// OS) shows no switch at all - only a muted "Disabled" with a "Why?" hint whose
// tooltip names the blocker. An already-enabled set can always be turned off, so
// it keeps its switch even if it could not be re-enabled from scratch.
function EnableControl({policySet, onToggle}) {
  const [busy, setBusy] = useState(false);
  const {enabled, canEnable, enableReason} = policySet;
  const blocked = !enabled && !canEnable;

  if (blocked) {
    return (
      <Tooltip title={enableReason}>
        <span
          onClick={(e) => e.stopPropagation()}
          style={{display: "inline-flex", alignItems: "center", gap: 8, cursor: "help"}}
        >
          <Text type="secondary" style={{fontSize: 13}}>Disabled</Text>
          <span style={{display: "inline-flex", alignItems: "center", gap: 4, color: "#1677ff", fontSize: 13}}>
            <QuestionCircleOutlined />
            Why?
          </span>
        </span>
      </Tooltip>
    );
  }

  const handleToggle = (checked, event) => {
    event?.stopPropagation?.();
    setBusy(true);
    Promise.resolve(onToggle(policySet, checked)).finally(() => setBusy(false));
  };

  return (
    <span
      onClick={(e) => e.stopPropagation()}
      style={{display: "inline-flex", alignItems: "center", gap: 8}}
    >
      <Switch size="small" checked={!!enabled} disabled={busy} loading={busy} onChange={handleToggle} />
      <Text style={{fontSize: 13}} type={enabled ? undefined : "secondary"}>{enabled ? "Enabled" : "Disabled"}</Text>
    </span>
  );
}

// enableRank orders cards by how live they are: enabled sets first, then sets
// that could be enabled right now, then sets this host cannot enforce at all.
function enableRank(policySet) {
  if (policySet.enabled) {
    return 0;
  }
  return policySet.canEnable ? 1 : 2;
}

function PolicySetCard({policySet, onOpen, onToggle}) {
  const strictness = policySet.strictness || "";

  return (
    <Col xs={24} sm={12} lg={8} xxl={6}>
      <Card
        hoverable
        style={{height: "100%", borderRadius: 12}}
        styles={{body: {padding: 20, height: "100%", display: "flex", flexDirection: "column"}}}
        onClick={() => onOpen(policySet)}
      >
        <div style={{display: "flex", alignItems: "flex-start", gap: 12, marginBottom: 12}}>
          <div style={{fontSize: 28, lineHeight: "34px", flexShrink: 0}}>
            <AgentIcon agent={policySet.agent} size={30} fallback={policySet.icon || <SafetyCertificateOutlined />} />
          </div>
          <div style={{flex: 1, minWidth: 0}}>
            <div style={{fontWeight: 600, fontSize: 16, lineHeight: "24px"}}>{policySet.displayName}</div>
            <Text type="secondary" style={{fontSize: 12}}>
              {countPolicyRules(policySet.policy)} rules
              {policySet.author ? ` · by ${policySet.author}` : ""}
            </Text>
          </div>
          {policySet.enabled && <Tag color="green" style={{margin: 0}}>Enabled</Tag>}
          {strictness && <Tag color={STRICTNESS_COLORS[strictness]} style={{margin: 0}}>{strictness}</Tag>}
        </div>

        <Paragraph type="secondary" ellipsis={{rows: 3}} style={{fontSize: 13, marginBottom: 12}}>
          {policySet.description}
        </Paragraph>

        <div style={{display: "flex", flexWrap: "wrap", gap: 4, marginTop: "auto"}}>
          {policySet.agent && <Tag color="blue" style={{margin: 0}}>{policySet.agent}</Tag>}
          {policySet.os && <Tag color="geekblue" style={{margin: 0}}>{policySet.os}</Tag>}
          {(policySet.tags || []).map((tag) => <Tag key={tag} style={{margin: 0}}>{tag}</Tag>)}
        </div>

        <div style={{marginTop: 14, display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8}}>
          <EnableControl policySet={policySet} onToggle={onToggle} />
          <span style={{color: "#1677ff", fontSize: 13, whiteSpace: "nowrap"}}>
            Open policy set <ArrowRightOutlined />
          </span>
        </div>
      </Card>
    </Col>
  );
}

export default function PolicyHubPage() {
  const history = useHistory();

  const [policySets, setPolicySets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const [search, setSearch] = useState("");
  const [agent, setAgent] = useState("");
  const [os, setOs] = useState("");
  const [strictness, setStrictness] = useState("");

  const load = useCallback(() => {
    return getPolicySets()
      .then((data) => {
        setPolicySets(data || []);
        setError(null);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  // Persist the toggle, then reload so the card reflects the server's decision
  // (which re-checks the same gate) rather than an optimistic guess.
  const onToggle = (policySet, enabled) => {
    return setPolicySetEnabled(policySet.name, enabled)
      .then(() => {
        message.success(enabled ? `Enabled ${policySet.displayName}` : `Disabled ${policySet.displayName}`);
        return load();
      })
      .catch((err) => message.error(err.message));
  };

  // Sort by enablement rank, keeping the server's name order within each rank
  // (Array.prototype.sort is stable), so enabled sets lead, ready-to-enable sets
  // follow, and sets this host cannot enforce sink to the bottom.
  const filtered = policySets
    .filter((policySet) => matches(policySet, {search, agent, os, strictness}))
    .sort((a, b) => enableRank(a) - enableRank(b));
  const isFiltered = !!(search || agent || os || strictness);

  const option = (label, values) => [
    {value: "", label: `${label}: All`},
    ...values.map((value) => ({value, label: value})),
  ];

  return (
    <div>
      <div style={{marginBottom: 20}}>
        <Title level={3} style={{marginBottom: 4}}>Policy Hub</Title>
        <Text type="secondary">
          Ready-made Casbin policy sets for AI agents. Each set guards one agent on one operating system
          across everything it does while you code. Open one to read its model, policy and example
          requests, and to see the decision each request gets, evaluated live in your browser.
        </Text>
      </div>

      {error && <Alert type="error" message={error} style={{marginBottom: 16}} />}

      <div style={{display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center", marginBottom: 16}}>
        <Input.Search
          placeholder="Search policy sets"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          allowClear
          style={{width: 240}}
        />
        <Select
          value={agent}
          onChange={setAgent}
          style={{minWidth: 190}}
          options={option("Agent", sortByOrder(uniqueValues(policySets, "agent"), AGENT_ORDER))}
        />
        <Select value={os} onChange={setOs} style={{minWidth: 160}} options={osOptions(policySets)} />
        <Select
          value={strictness}
          onChange={setStrictness}
          style={{minWidth: 170}}
          options={option("Strictness", STRICTNESS_ORDER.filter((value) => policySets.some((policySet) => policySet.strictness === value)))}
        />
        {isFiltered && (
          <Button onClick={() => {setSearch(""); setAgent(""); setOs(""); setStrictness("");}}>Reset</Button>
        )}
        <Text type="secondary" style={{fontSize: 13}}>
          {isFiltered ? `${filtered.length} / ${policySets.length}` : policySets.length} policy sets
        </Text>
      </div>

      {loading ? (
        <div style={{textAlign: "center", padding: "80px 0"}}><Spin size="large" /></div>
      ) : filtered.length === 0 ? (
        <Empty
          style={{marginTop: 60}}
          description={policySets.length === 0 ? "No policy sets yet - add a JSON file under data/policyhub" : "No policy set matches these filters"}
        />
      ) : (
        <Row gutter={[16, 16]}>
          {filtered.map((policySet) => (
            <PolicySetCard
              key={policySet.name}
              policySet={policySet}
              onOpen={() => history.push(`/policyhub/${policySet.name}`)}
              onToggle={onToggle}
            />
          ))}
        </Row>
      )}
    </div>
  );
}
