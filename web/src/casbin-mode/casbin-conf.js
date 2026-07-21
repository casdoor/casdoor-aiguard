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

// Ported from the Casbin editor (https://github.com/casbin/casbin-editor) so
// the model box highlights [sections], r/p/e/m definitions and matchers.
import {LanguageSupport, StreamLanguage} from "@codemirror/language";
import {tags as t} from "@lezer/highlight";

export const token = (stream, state) => {
  if (stream.sol()) {
    state.afterEqual = false;

    if (stream.match(/[rpem]\s*=/) || stream.match(/g\d*\s*=/)) {
      stream.backUp(1);
      return "builtin";
    }

    if (state.sec === "matchers" && (stream.match(/[rpem]\./) || stream.match(/g\d*\./))) {
      return "builtin";
    }
  }

  if (stream.match(/^\[.*?\]/)) {
    state.sec = stream.current().slice(1, -1);
    return "header";
  }

  if (stream.match(/#.*/)) {
    return "comment";
  }

  if (stream.eat("=")) {
    state.afterEqual = true;
    return null;
  }

  if (state.afterEqual || state.sec === "matchers") {
    if (state.sec === "request_definition" || state.sec === "policy_definition" || state.sec === "role_definition") {
      if (stream.match(/[a-zA-Z][a-zA-Z0-9]*/)) {return "property";}
    } else if (state.sec === "policy_effect") {
      if (stream.match(/some|where/)) {return "keyword";}
      if (stream.match(/allow|deny/)) {return "string";}
      if (stream.match(/p\./)) {return "builtin";}
      if (stream.match(/[a-zA-Z][a-zA-Z0-9]*/)) {return "property";}
    } else if (state.sec === "matchers") {
      if (stream.match(/[a-zA-Z_][a-zA-Z0-9_]*\(/)) {
        return "def";
      }
      if (stream.match(/[rpem](?=\.)/) || stream.match(/g\d*(?=\.)/)) {
        return "builtin";
      }
      if (stream.eat(".")) {
        return null;
      }
      if (stream.match(/[a-zA-Z][a-zA-Z0-9]*/)) {return "property";}
      if (stream.match(/==|!=|&&|\|\|/)) {return "operator";}
      if (stream.match(/"/)) {
        stream.skipTo("\"");
        stream.next();
        return "string";
      }
    }
  }

  stream.next();
  return null;
};

export const CasbinConfLang = StreamLanguage.define({
  name: "casbin-conf",
  startState: () => {
    return {sec: "", afterEqual: false};
  },
  token: token,
  blankLine: () => {},
  copyState: (state) => {
    return {...state};
  },
  indent: () => {
    return null;
  },
  languageData: {
    commentTokens: {line: "#"},
  },
  tokenTable: {
    header: t.heading,
    comment: t.lineComment,
    builtin: t.variableName,
    property: t.propertyName,
    keyword: t.keyword,
    string: t.string,
    operator: t.operator,
    def: t.function(t.variableName),
  },
});

export function CasbinConfSupport() {
  return new LanguageSupport(CasbinConfLang);
}
