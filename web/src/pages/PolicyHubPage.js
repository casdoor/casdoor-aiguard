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
import {Alert, Button, Card, Col, Empty, Input, Row, Select, Spin, Tag, Typography} from "antd";
import {ArrowRightOutlined, SafetyCertificateOutlined} from "@ant-design/icons";
import {useHistory} from "react-router-dom";
import {getPolicySets} from "../backend/api";
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

function PolicySetCard({policySet, onOpen}) {
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

        <div style={{marginTop: 14, color: "#1677ff", fontSize: 13}}>
          Open policy set <ArrowRightOutlined />
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

  useEffect(() => {
    getPolicySets()
      .then((data) => {
        setPolicySets(data || []);
        setError(null);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  const filtered = policySets.filter((policySet) => matches(policySet, {search, agent, os, strictness}));
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
            />
          ))}
        </Row>
      )}
    </div>
  );
}
