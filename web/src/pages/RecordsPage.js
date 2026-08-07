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

import React, {useCallback, useEffect, useMemo, useState} from "react";
import {Alert, Descriptions, Select, Space, Table, Tag, Tooltip, Typography, message} from "antd";
import {BulbOutlined, RobotOutlined} from "@ant-design/icons";
import {Link, useHistory, useLocation} from "react-router-dom";
import {getAgents, getRecords, setRecordFeedback} from "../backend/api";
import {AgentIcon} from "./policySetUtil";
import {recordsWithKeyInputs} from "./recordSummary";

const {Title, Text} = Typography;

const codeBlockStyle = {
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
  maxHeight: 300,
  overflow: "auto",
  background: "#f5f5f5",
  padding: 8,
  borderRadius: 4,
  margin: 0,
  fontFamily: "monospace",
  fontSize: 12,
};

// Payloads arrive as a JSON string; pretty-print when we can, show the raw text
// when we cannot, so a malformed payload is still inspectable.
function prettyObject(object) {
  try {
    return JSON.stringify(JSON.parse(object), null, 2);
  } catch {
    return object;
  }
}

function outcomeColor(outcome) {
  return {attempted: "processing", success: "success", failure: "error", denied: "warning"}[outcome] || "default";
}

function KeyInput({summary, expanded = false}) {
  if (!summary) {
    return <Text type="secondary">—</Text>;
  }
  if (expanded) {
    return (
      <Space direction="vertical" size={4} style={{display: "flex"}}>
        <Text type="secondary">{summary.label}</Text>
        <pre style={{...codeBlockStyle, maxHeight: 160}}>{summary.value}</pre>
      </Space>
    );
  }

  const singleLine = summary.value.replace(/\s+/g, " ");
  return (
    <Space direction="vertical" size={0} style={{display: "flex", minWidth: 0}}>
      <Text type="secondary" style={{fontSize: 12}}>{summary.label}</Text>
      <Tooltip
        placement="topLeft"
        overlayStyle={{maxWidth: 680}}
        title={(
          <span style={{display: "block", maxWidth: 640, maxHeight: 320, overflow: "auto", whiteSpace: "pre-wrap", wordBreak: "break-word"}}>
            {summary.value}
          </span>
        )}
      >
        <Text
          code
          style={{display: "block", maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap"}}
        >
          {singleLine}
        </Text>
      </Tooltip>
    </Space>
  );
}

function DecisionCell({record}) {
  const verdict = !record.correctable
    ? <Text type="secondary">logged</Text>
    : record.isAllowed
      ? <Tag color="green">allowed</Tag>
      : <Tag color="red">blocked</Tag>;

  return (
    <Space direction="vertical" size={2} style={{display: "flex", minWidth: 0}}>
      <Space size={4} wrap>
        {verdict}
        {record.isTriggered && <Tag color="red">policy matched</Tag>}
      </Space>
      {record.policySet && (
        <Link to={`/policyhub/${encodeURIComponent(record.policySet)}`} style={{overflow: "hidden", textOverflow: "ellipsis"}}>
          {record.policySet}
        </Link>
      )}
      {record.reason && (
        <Tooltip title={record.reason}>
          <Text type="danger" style={{display: "block", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap"}}>
            {record.reason}
          </Text>
        </Tooltip>
      )}
    </Space>
  );
}

// A record is aiguard's account of what happened, and the verdict on it is a
// guess made by rules somebody wrote in advance. FeedbackCell is where a person
// who knows better corrects that guess - and the correction does not stay a
// note: aiguard turns it into a Casbin rule on the Self-Learning page, so the
// same call is decided the corrected way next time.
function FeedbackCell({record, onChanged}) {
  const [saving, setSaving] = useState(false);

  // Only an operation the enforcer ruled on carries the Casbin triple a rule
  // would be written about; everything else was merely logged.
  if (!record.correctable) {
    return (
      <Tooltip title="This record was only logged, not ruled on, so there is no decision to correct.">
        <Text type="secondary">—</Text>
      </Tooltip>
    );
  }

  const change = (value) => {
    setSaving(true);
    setRecordFeedback(record.id, value)
      .then(() => {
        message.success(
          value === ""
            ? "Correction withdrawn - the rule it taught was forgotten"
            : `Learned: ${record.intent} on "${record.resource}" should ${value}`
        );
        onChanged();
      })
      .catch((err) => message.error(err.message))
      .finally(() => setSaving(false));
  };

  return (
    <Select
      size="small"
      style={{width: 150}}
      value={record.feedback || ""}
      loading={saving}
      disabled={saving}
      onChange={change}
      options={[
        {value: "", label: <Text type="secondary">verdict is right</Text>},
        {value: "allow", label: "should be allowed"},
        {value: "deny", label: "should be blocked"},
      ]}
    />
  );
}

function RecordDetail({record}) {
  return (
    <Descriptions bordered size="small" column={1} style={{margin: 8}}>
      <Descriptions.Item label="Record">{record.id}</Descriptions.Item>
      {record.resource && <Descriptions.Item label="Resource"><Text code>{record.resource}</Text></Descriptions.Item>}
      {record.intent && <Descriptions.Item label="Intent"><Text code>{record.intent}</Text></Descriptions.Item>}
      {record.feedback && (
        <Descriptions.Item label="Correction">
          <Space>
            <Tag color="gold" icon={<BulbOutlined />}>should {record.feedback}</Tag>
            <Text type="secondary">
              corrected by {record.feedbackBy}
              {record.feedbackTime ? ` on ${new Date(record.feedbackTime).toLocaleString()}` : ""} -
            </Text>
            <Link to="/self-learning">see the rule it taught</Link>
          </Space>
        </Descriptions.Item>
      )}
      {record.agentPath && <Descriptions.Item label="Agent path"><Text code>{record.agentPath}</Text></Descriptions.Item>}
      {record.user && <Descriptions.Item label="User">{record.user}</Descriptions.Item>}
      {record.channel && <Descriptions.Item label="Channel">{record.channel}</Descriptions.Item>}
      {record.sessionKey && <Descriptions.Item label="Session">{record.sessionKey}</Descriptions.Item>}
      {record.title && <Descriptions.Item label="Session title">{record.title}</Descriptions.Item>}
      {record.promptId && <Descriptions.Item label="Prompt ID"><Text code>{record.promptId}</Text></Descriptions.Item>}
      {record.toolUseId && <Descriptions.Item label="Tool use ID"><Text code>{record.toolUseId}</Text></Descriptions.Item>}
      {record.toolName && <Descriptions.Item label="Tool"><Text code>{record.toolName}</Text></Descriptions.Item>}
      {record.keyInput && (
        <Descriptions.Item label="Key input"><KeyInput summary={record.keyInput} expanded /></Descriptions.Item>
      )}
      {record.mcpServer && (
        <Descriptions.Item label="MCP target">
          <Text code>{record.mcpServer}{record.mcpTool ? ` / ${record.mcpTool}` : ""}</Text>
        </Descriptions.Item>
      )}
      {record.model && <Descriptions.Item label="Model"><Text code>{record.model}</Text></Descriptions.Item>}
      {record.durationMs ? <Descriptions.Item label="Duration">{record.durationMs.toLocaleString()} ms</Descriptions.Item> : null}
      {record.clientIp && <Descriptions.Item label="Reported from">{record.clientIp}</Descriptions.Item>}
      {record.detail && <Descriptions.Item label="Detail">{record.detail}</Descriptions.Item>}
      {record.isTriggered && <Descriptions.Item label="Triggered"><Tag color="red">policy matched</Tag></Descriptions.Item>}
      {record.policySet && (
        <Descriptions.Item label="Policy set">
          <Link to={`/policyhub/${encodeURIComponent(record.policySet)}`}>{record.policySet}</Link>
        </Descriptions.Item>
      )}
      {record.reason && <Descriptions.Item label="Reason">{record.reason}</Descriptions.Item>}
      {record.object && (
        <Descriptions.Item label="Payload">
          <pre style={codeBlockStyle}>{prettyObject(record.object)}</pre>
        </Descriptions.Item>
      )}
    </Descriptions>
  );
}

export default function RecordsPage() {
  const history = useHistory();
  const location = useLocation();
  // The agent filter lives in the URL, so a link from the agents table ("View
  // records for this agent") lands pre-filtered and the view stays shareable.
  const search = new URLSearchParams(location.search);
  const agent = search.get("agent") || "";
  const eventType = search.get("eventType") || "";
  const outcome = search.get("outcome") || "";
  const session = search.get("session") || "";

  const [records, setRecords] = useState([]);
  const [agents, setAgents] = useState([]);
  const [error, setError] = useState(null);

  useEffect(() => {
    getAgents()
      .then((data) => setAgents(data || []))
      .catch(() => setAgents([]));
  }, []);

  const load = useCallback(() => {
    getRecords(agent, 200, eventType, outcome, session)
      .then((data) => {
        setRecords(data || []);
        setError(null);
      })
      .catch((err) => setError(err.message));
  }, [agent, eventType, outcome, session]);

  useEffect(() => {
    load();
    const interval = setInterval(load, 3000);
    return () => clearInterval(interval);
  }, [load]);

  const agentOptions = [
    {value: "", label: "All agents"},
    ...Array.from(new Set(agents.map((item) => item.agentId)))
      .filter(Boolean)
      .map((id) => ({value: id, label: id})),
  ];

  const corrected = records.filter((record) => record.feedback).length;
  const displayedRecords = useMemo(() => recordsWithKeyInputs(records), [records]);

  const setFilter = (key, value) => {
    const params = new URLSearchParams(location.search);
    if (value) {
      params.set(key, value);
    } else {
      params.delete(key);
    }
    const query = params.toString();
    history.push(query ? `/records?${query}` : "/records");
  };

  const columns = [
    {title: "Time", dataIndex: "createdTime", key: "createdTime", width: 150, render: (v) => new Date(v).toLocaleString()},
    {
      title: "Agent",
      dataIndex: "agent",
      key: "agent",
      width: 145,
      render: (v, record) => (
        <Space direction="vertical" size={2} style={{display: "flex", minWidth: 0}}>
          <Tag color="blue" style={{display: "inline-flex", alignItems: "center", gap: 6, width: "fit-content"}}>
            <AgentIcon agent={v} size={16} fallback={<RobotOutlined />} />
            {v}
          </Tag>
          {(record.user || record.channel) && (
            <Tooltip title={[record.user, record.channel].filter(Boolean).join(" · ")}>
              <Text type="secondary" style={{display: "block", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap"}}>
                {[record.user, record.channel].filter(Boolean).join(" · ")}
              </Text>
            </Tooltip>
          )}
        </Space>
      ),
    },
    {
      title: "Event",
      key: "event",
      width: 150,
      render: (_, record) => (
        <Space size={4} wrap>
          <Tag>{record.eventType}</Tag>
          <Text code>{record.action}</Text>
          {record.outcome && <Tag color={outcomeColor(record.outcome)}>{record.outcome}</Tag>}
        </Space>
      ),
    },
    {
      title: "Target / model",
      key: "target",
      width: 150,
      render: (_, record) => {
        const target = record.mcpServer
          ? `${record.mcpServer}${record.mcpTool ? ` / ${record.mcpTool}` : ""}`
          : record.toolName;
        return (
          <Space direction="vertical" size={0}>
            {target && <Text code>{target}</Text>}
            {record.model && <Text type="secondary">{record.model}</Text>}
          </Space>
        );
      },
    },
    {
      title: (
        <Tooltip title="The most important argument for this tool call, such as a command, URL, query or file path.">
          <span>Key input</span>
        </Tooltip>
      ),
      key: "keyInput",
      width: 340,
      render: (_, record) => <KeyInput summary={record.keyInput} />,
    },
    {
      title: "Session",
      dataIndex: "sessionKey",
      key: "sessionKey",
      width: 170,
      ellipsis: true,
      render: (value, record) => (value ? (
        <Space direction="vertical" size={0} style={{display: "flex", minWidth: 0}}>
          {record.title && <Text strong ellipsis>{record.title}</Text>}
          <Link to={`/records?session=${encodeURIComponent(value)}`} style={{overflow: "hidden", textOverflow: "ellipsis"}}>
            {value}
          </Link>
        </Space>
      ) : null),
    },
    {
      title: "Decision",
      key: "decision",
      width: 160,
      render: (_, record) => <DecisionCell record={record} />,
    },
    {
      title: (
        <Tooltip title="Disagree with a verdict? Correct it here. Each correction becomes a Casbin rule on the Self-Learning page, so the same call is decided your way next time.">
          <Space size={4}><BulbOutlined />Feedback</Space>
        </Tooltip>
      ),
      key: "feedback",
      width: 170,
      render: (_, record) => <FeedbackCell record={record} onChanged={load} />,
    },
  ];

  return (
    <div>
      <Space style={{display: "flex", justifyContent: "space-between", marginBottom: 16}}>
        <Space align="center">
          <Title level={3} style={{margin: 0}}>Records</Title>
          {session &&
            <Tag closable onClose={(e) => { e.preventDefault(); setFilter("session", ""); }}>
              session <Text code>{session}</Text>
            </Tag>
          }
          {corrected > 0 &&
            <Link to="/self-learning">
              <Tag color="gold" icon={<BulbOutlined />} style={{cursor: "pointer"}}>
                {corrected} correction{corrected === 1 ? "" : "s"} learned
              </Tag>
            </Link>
          }
        </Space>
        <Space wrap>
          <Select
            value={eventType}
            onChange={(value) => setFilter("eventType", value)}
            style={{width: 160}}
            options={[
              {value: "", label: "All categories"},
              ...["session", "prompt", "llm", "tool", "mcp", "permission", "subagent", "compact"]
                .map((value) => ({value, label: value})),
            ]}
          />
          <Select
            value={outcome}
            onChange={(value) => setFilter("outcome", value)}
            style={{width: 150}}
            options={[
              {value: "", label: "All outcomes"},
              ...["attempted", "success", "failure", "denied"].map((value) => ({value, label: value})),
            ]}
          />
          <Select
            value={agent}
            options={agentOptions}
            onChange={(value) => setFilter("agent", value)}
            style={{width: 220}}
          />
        </Space>
      </Space>
      {error && <Alert type="error" message={error} style={{marginBottom: 16}} />}
      <Table
        rowKey="id"
        dataSource={displayedRecords}
        columns={columns}
        pagination={{pageSize: 20}}
        size="small"
        scroll={{x: 1400}}
        locale={{emptyText: "No records yet - patch an agent to start collecting them"}}
        expandable={{
          expandedRowRender: (record) => <RecordDetail record={record} />,
        }}
      />
    </div>
  );
}
