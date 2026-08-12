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

package patch

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// codexProviderTable is the one config.toml table aiguard owns.
const codexProviderTable = "model_providers." + codexProviderId

// codexTOMLSetting is one assignment aiguard manages. Value is already
// TOML-encoded (strconv.Quote for a string, "true" for a bool); an empty
// Value means "remove this key".
type codexTOMLSetting struct {
	Key   string
	Value string
}

// codexTOMLEdit is the complete set of changes a switcher wants made to
// config.toml, stated declaratively so editCodexTOML can touch only the lines
// those keys actually live on.
type codexTOMLEdit struct {
	// Root are top-level assignments, applied in order when they have to be
	// inserted rather than replaced.
	Root []codexTOMLSetting
	// Provider are the assignments inside [model_providers.aiguard].
	Provider []codexTOMLSetting
	// DropProviderTable removes [model_providers.aiguard] and everything under
	// it. It cannot be combined with Provider.
	DropProviderTable bool
}

func (e codexTOMLEdit) isEmpty() bool {
	return len(e.Root) == 0 && len(e.Provider) == 0 && !e.DropProviderTable
}

// editCodexTOML applies edit to the bytes of a config.toml, changing only the
// lines the managed keys live on.
//
// Marshaling the parsed document back out would be a tenth of this code, but
// it would also rewrite the whole file: comments, blank lines, key order and
// inline formatting all come back in go-toml's shape rather than the
// operator's. config.toml is a file people hand-maintain - MCP servers,
// profiles, sandbox settings - so aiguard edits it the way a person would and
// leaves every byte it does not own exactly where it was.
//
// The result is re-parsed and checked against edit before it is returned, so a
// line this editor misreads fails loudly instead of landing on disk.
func editCodexTOML(data []byte, edit codexTOMLEdit) ([]byte, error) {
	document, err := parseCodexTOML(data)
	if err != nil {
		return nil, err
	}
	updated, err := document.apply(edit)
	if err != nil {
		return nil, err
	}
	if err := verifyCodexTOML(updated, edit); err != nil {
		return nil, err
	}
	return updated, nil
}

// codexTOMLDocument is config.toml split into lines, with every table header
// and assignment located. Edits are expressed as replacements, removals and
// insertions against those line indexes.
type codexTOMLDocument struct {
	lines   []string // each line keeps its own ending
	newline string
	tables  []codexTOMLTable
	entries []codexTOMLEntry
	rootEnd int // one past the last line of the root table's region
}

// codexTOMLEntry is one assignment, spanning lines [start, end) so a value
// continued across several lines is removed - or rejected - as a whole.
type codexTOMLEntry struct {
	table string
	key   string
	start int
	end   int
}

// codexTOMLTable is one header and the lines [header, end) beneath it. Arrays
// of tables are marked, so [[model_providers.aiguard]] can never be mistaken
// for the table aiguard owns.
type codexTOMLTable struct {
	name   string
	array  bool
	header int
	end    int
}

// parseCodexTOML locates the structure editCodexTOML needs. It rejects a file
// go-toml cannot parse first: aiguard only edits config.toml files it can also
// read back, so a hand-broken one is reported rather than compounded.
func parseCodexTOML(data []byte) (*codexTOMLDocument, error) {
	if strings.TrimSpace(string(data)) != "" {
		probe := map[string]any{}
		if err := toml.Unmarshal(data, &probe); err != nil {
			return nil, fmt.Errorf("cannot parse Codex config.toml: %w", err)
		}
	}

	document := &codexTOMLDocument{
		lines:   splitTOMLLines(string(data)),
		newline: detectTOMLNewline(string(data)),
	}
	document.rootEnd = len(document.lines)

	table := ""
	for index := 0; index < len(document.lines); index++ {
		body, _ := splitTOMLLineEnding(document.lines[index])
		trimmed := strings.TrimSpace(body)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if name, array, ok := tomlTableHeader(body); ok {
			if count := len(document.tables); count == 0 {
				document.rootEnd = index
			} else {
				document.tables[count-1].end = index
			}
			document.tables = append(document.tables, codexTOMLTable{
				name: name, array: array, header: index, end: len(document.lines),
			})
			// An array-of-tables entry gets a name no managed table can equal,
			// so its assignments are never mistaken for the real table's.
			table = name
			if array {
				table = "[[" + name
			}
			continue
		}

		key, valueStart, ok := tomlAssignment(body)
		if !ok {
			continue
		}
		// A multi-line array or string keeps running until its brackets or
		// quotes close, and nothing inside it is structure.
		state := scanTOMLValue(tomlScanState{}, body[valueStart:])
		end := index + 1
		for state.continuing() && end < len(document.lines) {
			next, _ := splitTOMLLineEnding(document.lines[end])
			state = scanTOMLValue(state, next)
			end++
		}
		document.entries = append(document.entries, codexTOMLEntry{
			table: table, key: key, start: index, end: end,
		})
		index = end - 1
	}
	return document, nil
}

func (d *codexTOMLDocument) entryOf(table, key string) (codexTOMLEntry, bool) {
	for _, entry := range d.entries {
		if entry.table == table && entry.key == key {
			return entry, true
		}
	}
	return codexTOMLEntry{}, false
}

func (d *codexTOMLDocument) tableOf(name string) (codexTOMLTable, bool) {
	for _, table := range d.tables {
		if !table.array && table.name == name {
			return table, true
		}
	}
	return codexTOMLTable{}, false
}

// apply resolves edit into line-level changes and renders the result. Every
// change is collected against the original line indexes first and only applied
// during rendering, so nothing shifts underneath a later lookup.
func (d *codexTOMLDocument) apply(edit codexTOMLEdit) ([]byte, error) {
	if edit.DropProviderTable && len(edit.Provider) != 0 {
		return nil, fmt.Errorf("cannot both set and drop [%s] in one edit", codexProviderTable)
	}

	removed := map[int]bool{}
	inserted := map[int][]string{}

	missingRoot, err := d.resolve(edit.Root, "", removed)
	if err != nil {
		return nil, err
	}

	table, hasTable := d.tableOf(codexProviderTable)
	if edit.DropProviderTable && hasTable {
		// The blank lines separating the table from what came before it go with
		// it, the way deleting the block by hand would take them - otherwise a
		// switch to a provider and back leaves a blank line behind every time.
		start := table.header
		for start > 0 {
			body, _ := splitTOMLLineEnding(d.lines[start-1])
			if strings.TrimSpace(body) != "" {
				break
			}
			start--
		}
		for index := start; index < table.end; index++ {
			removed[index] = true
		}
	}
	if len(edit.Provider) != 0 && hasTable {
		missingProvider, err := d.resolve(edit.Provider, codexProviderTable, removed)
		if err != nil {
			return nil, err
		}
		if len(missingProvider) != 0 {
			d.terminateLine(table.header)
			inserted[table.header+1] = append(inserted[table.header+1], missingProvider...)
		}
	}

	// Root keys go in before a freshly appended provider table, so the two
	// land in the file in that order when both are inserted at the very end.
	if len(missingRoot) != 0 {
		at := d.rootInsertPoint()
		if at == len(d.lines) {
			d.terminateLastLine()
		} else if body, _ := splitTOMLLineEnding(d.lines[at]); strings.TrimSpace(body) != "" {
			// Keep the inserted keys off the table header they now precede.
			missingRoot = append(missingRoot, d.newline)
		}
		inserted[at] = append(inserted[at], missingRoot...)
	}

	if len(edit.Provider) != 0 && !hasTable {
		block := []string{}
		if len(d.lines) != 0 {
			d.terminateLastLine()
			if strings.TrimSpace(d.lines[len(d.lines)-1]) != "" {
				block = append(block, d.newline)
			}
		} else if len(missingRoot) != 0 {
			block = append(block, d.newline)
		}
		block = append(block, "["+codexProviderTable+"]"+d.newline)
		for _, setting := range edit.Provider {
			if setting.Value != "" {
				block = append(block, setting.Key+" = "+setting.Value+d.newline)
			}
		}
		inserted[len(d.lines)] = append(inserted[len(d.lines)], block...)
	}

	return d.render(removed, inserted), nil
}

// resolve replaces or removes each setting that is already in table and
// returns the rendered lines for the ones that have to be inserted.
func (d *codexTOMLDocument) resolve(settings []codexTOMLSetting, table string, removed map[int]bool) ([]string, error) {
	where := "top-level"
	if table != "" {
		where = "[" + table + "]"
	}

	var missing []string
	for _, setting := range settings {
		entry, found := d.entryOf(table, setting.Key)
		switch {
		case setting.Value == "":
			if found {
				for index := entry.start; index < entry.end; index++ {
					removed[index] = true
				}
			}
		case !found:
			missing = append(missing, setting.Key+" = "+setting.Value+d.newline)
		default:
			if entry.end != entry.start+1 {
				return nil, fmt.Errorf("cannot edit %s %s: its value spans several lines", where, setting.Key)
			}
			line, err := replaceTOMLValue(d.lines[entry.start], setting.Value)
			if err != nil {
				return nil, fmt.Errorf("cannot edit %s %s: %w", where, setting.Key, err)
			}
			d.lines[entry.start] = line
		}
	}
	return missing, nil
}

// rootInsertPoint is where a missing top-level key goes: after the last line
// of the root region, but before any comment block introducing the table
// header that follows it.
func (d *codexTOMLDocument) rootInsertPoint() int {
	at := d.rootEnd
	for at > 0 {
		body, _ := splitTOMLLineEnding(d.lines[at-1])
		trimmed := strings.TrimSpace(body)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			break
		}
		at--
	}
	return at
}

// terminateLine gives a line an ending, so an insertion after it starts on a
// line of its own. A file whose last line has no newline is the usual case.
func (d *codexTOMLDocument) terminateLine(index int) {
	if !strings.HasSuffix(d.lines[index], "\n") {
		d.lines[index] += d.newline
	}
}

func (d *codexTOMLDocument) terminateLastLine() {
	if last := len(d.lines) - 1; last >= 0 {
		d.terminateLine(last)
	}
}

func (d *codexTOMLDocument) render(removed map[int]bool, inserted map[int][]string) []byte {
	out := make([]string, 0, len(d.lines)+8)
	for index := 0; index <= len(d.lines); index++ {
		out = append(out, inserted[index]...)
		if index < len(d.lines) && !removed[index] {
			out = append(out, d.lines[index])
		}
	}
	return []byte(strings.Join(out, ""))
}

// replaceTOMLValue swaps in a new value for a single-line assignment, keeping
// the line's own spacing around "=" and any trailing comment.
func replaceTOMLValue(line, value string) (string, error) {
	body, ending := splitTOMLLineEnding(line)
	_, start, ok := tomlAssignment(body)
	if !ok {
		return "", fmt.Errorf("the assignment has no =")
	}
	for start < len(body) && (body[start] == ' ' || body[start] == '\t') {
		start++
	}
	end := len(body)
	if comment := tomlCommentStart(body, start); comment >= 0 {
		end = comment
	}
	for end > start && (body[end-1] == ' ' || body[end-1] == '\t') {
		end--
	}
	return body[:start] + value + body[end:] + ending, nil
}

// verifyCodexTOML re-reads the edited document and checks it says what edit
// asked for. This is the safety net under the line editor: any line it
// misclassified shows up here as a wrong or duplicated key, and the caller
// aborts with the original file still on disk.
func verifyCodexTOML(data []byte, edit codexTOMLEdit) error {
	settings := map[string]any{}
	if err := toml.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("the edited Codex config.toml no longer parses: %w", err)
	}

	for _, setting := range edit.Root {
		if err := verifyTOMLSetting(settings, setting, "top-level"); err != nil {
			return err
		}
	}

	providers, _ := settings["model_providers"].(map[string]any)
	provider, hasProvider := providers[codexProviderId].(map[string]any)
	if edit.DropProviderTable {
		if hasProvider {
			return fmt.Errorf("the edited Codex config.toml still defines [%s]", codexProviderTable)
		}
		return nil
	}
	if len(edit.Provider) == 0 {
		return nil
	}
	if !hasProvider {
		return fmt.Errorf("the edited Codex config.toml does not define [%s]", codexProviderTable)
	}
	for _, setting := range edit.Provider {
		if err := verifyTOMLSetting(provider, setting, "["+codexProviderTable+"]"); err != nil {
			return err
		}
	}
	return nil
}

func verifyTOMLSetting(table map[string]any, setting codexTOMLSetting, where string) error {
	actual, present := table[setting.Key]
	if setting.Value == "" {
		if present {
			return fmt.Errorf("the edited Codex config.toml still has %s %s", where, setting.Key)
		}
		return nil
	}
	expected, err := decodeTOMLValue(setting.Value)
	if err != nil {
		return err
	}
	if !present || !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("the edited Codex config.toml has %s %s = %v, want %v", where, setting.Key, actual, expected)
	}
	return nil
}

// decodeTOMLValue parses an already-encoded value the way go-toml will read it
// back, so verifyTOMLSetting can compare like with like instead of guessing at
// the Go type each TOML literal decodes to.
func decodeTOMLValue(encoded string) (any, error) {
	probe := map[string]any{}
	if err := toml.Unmarshal([]byte("value = "+encoded), &probe); err != nil {
		return nil, fmt.Errorf("cannot use %q as a TOML value: %w", encoded, err)
	}
	return probe["value"], nil
}

// tomlScanState is the part of TOML lexing that carries from one line to the
// next: an open multi-line string, or a value whose brackets are still open.
type tomlScanState struct {
	multiline string
	depth     int
}

func (s tomlScanState) continuing() bool { return s.multiline != "" || s.depth > 0 }

// scanTOMLValue advances state across one line of a value, so the caller can
// tell where a multi-line array or string really ends - and therefore that a
// "[1, 2]," inside one is not a table header. It does no validation: an
// unparseable file is rejected before any of this runs.
func scanTOMLValue(state tomlScanState, text string) tomlScanState {
	for index := 0; index < len(text); {
		if state.multiline != "" {
			if strings.HasPrefix(text[index:], state.multiline) {
				index += len(state.multiline)
				state.multiline = ""
				continue
			}
			index++
			continue
		}
		switch {
		case strings.HasPrefix(text[index:], `"""`):
			state.multiline = `"""`
			index += 3
		case strings.HasPrefix(text[index:], `'''`):
			state.multiline = `'''`
			index += 3
		case text[index] == '"':
			index = skipTOMLString(text, index, '"', true)
		case text[index] == '\'':
			index = skipTOMLString(text, index, '\'', false)
		case text[index] == '#':
			return state
		case text[index] == '[' || text[index] == '{':
			state.depth++
			index++
		case text[index] == ']' || text[index] == '}':
			if state.depth > 0 {
				state.depth--
			}
			index++
		default:
			index++
		}
	}
	return state
}

// skipTOMLString returns the index just past a single-line string starting at
// start. An unterminated one runs to the end of the line, which is what TOML
// says of it too.
func skipTOMLString(text string, start int, quote byte, escapes bool) int {
	for index := start + 1; index < len(text); index++ {
		if escapes && text[index] == '\\' {
			index++
			continue
		}
		if text[index] == quote {
			return index + 1
		}
	}
	return len(text)
}

// tomlTableHeader reports the table a "[name]" or "[[name]]" line opens.
func tomlTableHeader(body string) (string, bool, bool) {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "[") {
		return "", false, false
	}
	array := strings.HasPrefix(trimmed, "[[")
	open, closing := 1, "]"
	if array {
		open, closing = 2, "]]"
	}

	end := -1
	for index := open; index < len(trimmed) && end < 0; index++ {
		switch trimmed[index] {
		case '"':
			index = skipTOMLString(trimmed, index, '"', true) - 1
		case '\'':
			index = skipTOMLString(trimmed, index, '\'', false) - 1
		case ']':
			end = index
		}
	}
	if end < 0 || !strings.HasPrefix(trimmed[end:], closing) {
		return "", false, false
	}
	if rest := strings.TrimSpace(trimmed[end+len(closing):]); rest != "" && !strings.HasPrefix(rest, "#") {
		return "", false, false
	}
	return strings.TrimSpace(trimmed[open:end]), array, true
}

// tomlAssignment splits "key = value" at its top-level "=", returning the key
// and where the value starts.
func tomlAssignment(body string) (string, int, bool) {
	for index := 0; index < len(body); index++ {
		switch body[index] {
		case '"':
			index = skipTOMLString(body, index, '"', true) - 1
		case '\'':
			index = skipTOMLString(body, index, '\'', false) - 1
		case '#':
			return "", 0, false
		case '=':
			key := unquoteTOMLKey(strings.TrimSpace(body[:index]))
			if key == "" {
				return "", 0, false
			}
			return key, index + 1, true
		}
	}
	return "", 0, false
}

// unquoteTOMLKey strips the quotes off a single quoted key, so `"model" = ...`
// is recognised as the same managed key `model = ...` is. A dotted key is left
// as written and simply never matches one - verifyCodexTOML catches the
// duplicate that would result rather than letting it reach disk.
func unquoteTOMLKey(key string) string {
	if len(key) >= 2 && (key[0] == '"' || key[0] == '\'') && key[len(key)-1] == key[0] {
		if inner := key[1 : len(key)-1]; !strings.ContainsAny(inner, `"'.`) {
			return inner
		}
	}
	return key
}

// tomlCommentStart is the index of the "#" that starts a trailing comment, or
// -1 when the rest of the line is all value.
func tomlCommentStart(text string, start int) int {
	for index := start; index < len(text); index++ {
		switch text[index] {
		case '"':
			index = skipTOMLString(text, index, '"', true) - 1
		case '\'':
			index = skipTOMLString(text, index, '\'', false) - 1
		case '#':
			return index
		}
	}
	return -1
}

// splitTOMLLines splits a document into lines that each keep their own ending,
// so rendering is a plain concatenation and CRLF files stay CRLF.
func splitTOMLLines(document string) []string {
	if document == "" {
		return nil
	}
	lines := strings.SplitAfter(document, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// detectTOMLNewline picks the ending inserted lines should use: whatever the
// file already uses, defaulting to "\n" for a file with no lines yet.
func detectTOMLNewline(document string) string {
	if strings.Contains(document, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func splitTOMLLineEnding(line string) (string, string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}
