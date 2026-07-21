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
import {Alert, Descriptions, Select, Space, Table, Tag, Typography} from "antd";
import {useHistory, useLocation} from "react-router-dom";
import {getAgents, getRecords} from "../backend/api";

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

function RecordDetail({record}) {
  return (
    <Descriptions bordered size="small" column={1} style={{margin: 8}}>
      <Descriptions.Item label="Record">{record.id}</Descriptions.Item>
      {record.agentPath && <Descriptions.Item label="Agent path"><Text code>{record.agentPath}</Text></Descriptions.Item>}
      {record.sessionKey && <Descriptions.Item label="Session">{record.sessionKey}</Descriptions.Item>}
      {record.clientIp && <Descriptions.Item label="Reported from">{record.clientIp}</Descriptions.Item>}
      {record.detail && <Descriptions.Item label="Detail">{record.detail}</Descriptions.Item>}
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
  const agent = new URLSearchParams(location.search).get("agent") || "";

  const [records, setRecords] = useState([]);
  const [agents, setAgents] = useState([]);
  const [error, setError] = useState(null);

  useEffect(() => {
    getAgents()
      .then((data) => setAgents(data || []))
      .catch(() => setAgents([]));
  }, []);

  useEffect(() => {
    const load = () => {
      getRecords(agent, 200)
        .then((data) => {
          setRecords(data || []);
          setError(null);
        })
        .catch((err) => setError(err.message));
    };
    load();
    const interval = setInterval(load, 3000);
    return () => clearInterval(interval);
  }, [agent]);

  const agentOptions = [
    {value: "", label: "All agents"},
    ...Array.from(new Set(agents.map((item) => item.agentId)))
      .filter(Boolean)
      .map((id) => ({value: id, label: id})),
  ];

  const setAgent = (value) => {
    history.push(value ? `/records?agent=${encodeURIComponent(value)}` : "/records");
  };

  const columns = [
    {title: "Time", dataIndex: "createdTime", key: "createdTime", render: (v) => new Date(v).toLocaleString()},
    {title: "Agent", dataIndex: "agent", key: "agent", render: (v) => <Tag color="blue">{v}</Tag>},
    {title: "Event", key: "event", render: (_, record) => <Text code>{`${record.eventType}:${record.action}`}</Text>},
    {title: "Channel", dataIndex: "channel", key: "channel"},
    {title: "User", dataIndex: "user", key: "user"},
    {title: "Session", dataIndex: "sessionKey", key: "sessionKey", ellipsis: true},
    {
      title: "Triggered",
      dataIndex: "isTriggered",
      key: "isTriggered",
      render: (value) => (value ? <Tag color="red">yes</Tag> : null),
    },
  ];

  return (
    <div>
      <Space style={{display: "flex", justifyContent: "space-between", marginBottom: 16}}>
        <Title level={3} style={{margin: 0}}>Records</Title>
        <Select
          value={agent}
          options={agentOptions}
          onChange={setAgent}
          style={{width: 220}}
        />
      </Space>
      {error && <Alert type="error" message={error} style={{marginBottom: 16}} />}
      <Table
        rowKey="id"
        dataSource={records}
        columns={columns}
        pagination={{pageSize: 20}}
        size="small"
        locale={{emptyText: "No records yet - patch an agent to start collecting them"}}
        expandable={{
          expandedRowRender: (record) => <RecordDetail record={record} />,
        }}
      />
    </div>
  );
}
