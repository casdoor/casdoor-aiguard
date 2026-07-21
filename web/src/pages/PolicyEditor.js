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

// The two ways a policy set is shown anywhere in aiguard: the three editors it
// is written in, and the table of what its example requests decide. A policy
// set from the Policy Hub and a person's own digital employee set are the same
// three text blocks, so both pages render them with the code here.

import React from "react";
import {Card, Table, Tag, Typography} from "antd";
import CodeMirror from "@uiw/react-codemirror";
import {EditorView} from "@codemirror/view";
import {CasbinConfSupport} from "../casbin-mode/casbin-conf";
import {CasbinPolicySupport} from "../casbin-mode/casbin-csv";
import {monospace} from "./policySetUtil";

const {Text} = Typography;

export const editorStyle = {fontFamily: monospace, fontSize: 12};

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

const readOnlyTheme = EditorView.theme({
  "&": {backgroundColor: "#fafafa"},
  ".cm-content": {caretColor: "transparent"},
});

// A fused set is generated rather than written, so its editors are read-only:
// there is nothing to save, and editing them would only diverge from the two
// sets they were derived from.
//
// `rows` sets the height the editor starts at; `maxHeight` caps how far it
// grows, which is what lets the same editor appear somewhere that cannot give
// it the whole page - the policy previews on the fusion cards.
export function PolicyEditor({title, extra, value, rows, maxHeight, onChange, language, readOnly = false}) {
  const extensions = [cmTheme, EditorView.lineWrapping];
  if (LANGUAGE_EXTENSIONS[language]) {
    extensions.push(LANGUAGE_EXTENSIONS[language]);
  }
  if (readOnly) {
    extensions.push(readOnlyTheme, EditorView.editable.of(false));
  }

  return (
    <Card size="small" title={title} extra={extra} style={{height: "100%"}} styles={{body: {padding: 0}}}>
      <CodeMirror
        value={value}
        onChange={onChange}
        extensions={extensions}
        minHeight={`${rows * 1.6}em`}
        maxHeight={maxHeight}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLine: !readOnly,
          bracketMatching: true,
          foldGutter: false,
          highlightActiveLineGutter: !readOnly,
        }}
      />
    </Card>
  );
}

// DecisionTag renders one verdict the same way everywhere it appears: in a
// single set's result table and in each column of the fusion comparison.
export function DecisionTag({row}) {
  if (row.error) {
    return <Tag color="red">error</Tag>;
  }
  if (row.skipped) {
    return <Tag>skipped</Tag>;
  }
  return <Tag color={row.allowed ? "green" : "red"}>{row.allowed ? "allow" : "deny"}</Tag>;
}

// The result table mirrors the Casbin editor: one row per request line, with
// the rule that decided it so a surprising verdict is traceable.
export function ResultTable({results}) {
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
      render: (_, row) => <DecisionTag row={row} />,
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
