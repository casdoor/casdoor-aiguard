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

// Runs a policy set entirely in the browser with node-casbin, the same engine
// the Casbin editor uses, so the Policy Hub can show a live verdict for every
// example request without a round trip to aiguard.

import {DefaultRoleManager, StringAdapter, newEnforcer, newModel} from "casbin";

// parseRequest splits one request line into enforcer arguments. Commas inside
// braces belong to an ABAC attribute object, so "{a: 1, b: 2}, read" is two
// arguments, not three.
export function parseRequest(line) {
  const values = [];
  let value = "";
  let depth = 0;
  let isObject = false;

  const push = () => {
    values.push(isObject ? toObject(value) : value.trim());
    value = "";
    isObject = false;
  };

  for (const char of line) {
    if (depth === 0 && char === ",") {
      push();
      continue;
    }

    value += char;
    if (char === "{") {
      depth++;
      isObject = true;
    } else if (char === "}") {
      depth--;
    }
  }

  if (depth !== 0) {
    throw new Error(`unbalanced braces in request: ${line}`);
  }
  if (value !== "") {
    push();
  }
  return values;
}

// Attribute objects are written as JavaScript literals ({sub: "alice"}), the
// same shape the Casbin editor accepts, so they are evaluated rather than
// JSON-parsed. Everything here is local: the policy sets come from aiguard's
// own data directory and the requests from the textarea next to the result.
function toObject(text) {
  // eslint-disable-next-line no-new-func
  return new Function(`return (${text});`)();
}

// newPolicyEnforcer builds the enforcer one policy set runs on. Callers that
// evaluate several sets against the same requests - policy fusion does - build
// one enforcer per set and keep it for every line, rather than recompiling the
// model for each.
export async function newPolicyEnforcer({model, policy}) {
  const enforcer = await newEnforcer(newModel(model), (policy || "").trim() ? new StringAdapter(policy) : undefined);
  if (!enforcer.getRoleManager()) {
    enforcer.setRoleManager(new DefaultRoleManager(10));
  }
  return enforcer;
}

// enforce evaluates every non-empty request line against the model and policy,
// returning one row per line: the request, whether it was allowed, and the
// policy rule that decided it (empty when nothing matched).
export async function enforce({model, policy, request}) {
  const enforcer = await newPolicyEnforcer({model, policy});

  const lines = (request || "").split("\n").filter((line) => line.trim() !== "");

  return Promise.all(lines.map(async(line) => {
    if (line.trim().startsWith("#")) {
      return {request: line, skipped: true};
    }

    try {
      const [allowed, reason] = await enforcer.enforceEx(...parseRequest(line));
      return {request: line, allowed, reason};
    } catch (error) {
      return {request: line, error: error.message};
    }
  }));
}
