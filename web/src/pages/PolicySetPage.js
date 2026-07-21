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
import {Alert, Button, Card, Col, Row, Space, Spin, Table, Tag, Typography} from "antd";
import {ArrowLeftOutlined, PlayCircleOutlined, ReloadOutlined} from "@ant-design/icons";
import {useHistory, useParams} from "react-router-dom";
import CodeMirror from "@uiw/react-codemirror";
import {EditorView} from "@codemirror/view";
import {getPolicySet} from "../backend/api";
import {enforce} from "../utils/casbin";
import {CasbinConfSupport} from "../casbin-mode/casbin-conf";
import {CasbinPolicySupport} from "../casbin-mode/casbin-csv";
import {AgentIcon, STRICTNESS_COLORS, countPolicyRules, monospace} from "./policySetUtil";

const {Title, Text, Paragraph} = Typography;

const editorStyle = {fontFamily: monospace, fontSize: 12};

// Reuse the Casbin editor's stream modes so [sections], r/p/e/m tokens and the
// comma-separated policy/request values are highlighted the same way here.
const LANGUAGE_EXTENSIONS = {
  conf: CasbinConfSupport(),
  csv: CasbinPolicySupport(),
};

const cmTheme = EditorView.theme({
  "&": {fontSize: "12px", backgroundColor: "#fff"},
  ".cm-content": {fontFamily: monospace},
  ".cm-gutters": {fontFamily: monospace, backgroundColor: "#fafafa", border: "none"},
  "&.cm-focused": {outline: "none"},
});

function Editor({title, extra, value, rows, onChange, language}) {
  const extensions = [cmTheme, EditorView.lineWrapping];
  if (LANGUAGE_EXTENSIONS[language]) {
    extensions.push(LANGUAGE_EXTENSIONS[language]);
  }

  return (
    <Card size="small" title={title} extra={extra} style={{height: "100%"}} styles={{body: {padding: 0}}}>
      <CodeMirror
        value={value}
        onChange={onChange}
        extensions={extensions}
        minHeight={`${rows * 1.6}em`}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLine: true,
          bracketMatching: true,
          foldGutter: false,
          highlightActiveLineGutter: true,
        }}
      />
    </Card>
  );
}

// The result table mirrors the Casbin editor: one row per request line, with
// the rule that decided it so a surprising verdict is traceable.
function ResultTable({results}) {
  const columns = [
    {
      title: "Request",
      dataIndex: "request",
      key: "request",
      render: (value) => <Text style={editorStyle}>{value}</Text>,
    },
    {
      title: "Decision",
      key: "decision",
      width: 120,
      render: (_, row) => {
        if (row.error) {
          return <Tag color="red">error</Tag>;
        }
        if (row.skipped) {
          return <Tag>skipped</Tag>;
        }
        return <Tag color={row.allowed ? "green" : "red"}>{row.allowed ? "allow" : "deny"}</Tag>;
      },
    },
    {
      title: "Matched rule",
      key: "reason",
      render: (_, row) => {
        if (row.error) {
          return <Text type="danger" style={editorStyle}>{row.error}</Text>;
        }
        if (row.skipped) {
          return <Text type="secondary" style={editorStyle}>comment, not evaluated</Text>;
        }
        if (row.reason && row.reason.length > 0) {
          return <Text style={editorStyle}>p, {row.reason.join(", ")}</Text>;
        }
        return <Text type="secondary" style={editorStyle}>no rule matched</Text>;
      },
    },
  ];

  return <Table rowKey={(row, index) => index} dataSource={results} columns={columns} pagination={false} size="small" />;
}

export default function PolicySetPage() {
  const {name} = useParams();
  const history = useHistory();

  const [policySet, setPolicySet] = useState(null);
  const [model, setModel] = useState("");
  const [policy, setPolicy] = useState("");
  const [request, setRequest] = useState("");

  const [results, setResults] = useState([]);
  const [error, setError] = useState(null);
  const [loadError, setLoadError] = useState(null);
  const [running, setRunning] = useState(false);

  const reset = useCallback((data) => {
    setModel(data.model || "");
    setPolicy(data.policy || "");
    setRequest(data.request || "");
  }, []);

  useEffect(() => {
    getPolicySet(name)
      .then((data) => {
        setPolicySet(data);
        reset(data);
      })
      .catch((err) => setLoadError(err.message));
  }, [name, reset]);

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

  if (loadError) {
    return (
      <div>
        <Button icon={<ArrowLeftOutlined />} onClick={() => history.push("/policyhub")}>Back to Policy Hub</Button>
        <Alert type="error" message={loadError} style={{marginTop: 16}} />
      </div>
    );
  }

  if (!policySet) {
    return <div style={{textAlign: "center", padding: "80px 0"}}><Spin size="large" /></div>;
  }

  const allowed = results.filter((row) => row.allowed).length;
  const evaluated = results.filter((row) => !row.skipped).length;

  return (
    <div>
      <Button type="link" icon={<ArrowLeftOutlined />} style={{paddingLeft: 0}} onClick={() => history.push("/policyhub")}>
        Policy Hub
      </Button>

      <div style={{display: "flex", alignItems: "flex-start", gap: 12, marginTop: 8}}>
        <div style={{fontSize: 32, lineHeight: "40px"}}>
          <AgentIcon agent={policySet.agent} size={34} fallback={policySet.icon} />
        </div>
        <div style={{flex: 1}}>
          <Space align="center" wrap>
            <Title level={3} style={{margin: 0}}>{policySet.displayName}</Title>
            {policySet.strictness && <Tag color={STRICTNESS_COLORS[policySet.strictness]}>{policySet.strictness}</Tag>}
          </Space>
          <Paragraph type="secondary" style={{marginTop: 8, marginBottom: 8, maxWidth: 900}}>
            {policySet.description}
          </Paragraph>
          <Space wrap size={4}>
            {policySet.agent && <Tag color="blue" style={{margin: 0}}>{policySet.agent}</Tag>}
            {policySet.os && <Tag color="geekblue" style={{margin: 0}}>{policySet.os}</Tag>}
            {(policySet.tags || []).map((tag) => <Tag key={tag} style={{margin: 0}}>{tag}</Tag>)}
          </Space>
        </div>
        <Button icon={<ReloadOutlined />} onClick={() => reset(policySet)}>Reset changes</Button>
      </div>

      <Row gutter={[16, 16]} style={{marginTop: 20}}>
        <Col xs={24} lg={12}>
          <Editor title="Casbin model" value={model} rows={18} onChange={setModel} language="conf" />
        </Col>
        <Col xs={24} lg={12}>
          <Row gutter={[16, 16]}>
            <Col span={24}>
              <Editor
                title="Policy"
                extra={<Text type="secondary" style={{fontSize: 12}}>{countPolicyRules(policy)} rules</Text>}
                value={policy}
                rows={9}
                onChange={setPolicy}
                language="csv"
              />
            </Col>
            <Col span={24}>
              <Editor
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
