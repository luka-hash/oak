package main

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// byte slice helpers from the Ink interpreter source code,
// github.com/thesephist/ink

// zero-extend a slice of bytes to given length
func zeroExtend(s []byte, max int) []byte {
	if max <= len(s) {
		return s
	}

	extended := make([]byte, max)
	copy(extended, s)
	return extended
}

// return the max length of two slices
func maxLen(a, b []byte) int {
	if alen, blen := len(a), len(b); alen < blen {
		return blen
	} else {
		return alen
	}
}

type Value interface {
	String() string
	Eq(Value) bool
}

type EmptyValue byte

// interned "empty" value
const empty EmptyValue = 0

func (v EmptyValue) String() string {
	return "_"
}
func (v EmptyValue) Eq(u Value) bool {
	return true
}

// Null need not contain any data, so we use the most compact data
// representation we can.
type NullValue byte

// interned "null"
const null NullValue = 0

func (v NullValue) String() string {
	return "?"
}
func (v NullValue) Eq(u Value) bool {
	if _, ok := u.(EmptyValue); ok {
		return true
	}

	if _, ok := u.(NullValue); ok {
		return true
	}
	return false
}

type StringValue []byte

var emptyString = StringValue("")

func (v StringValue) String() string {
	return fmt.Sprintf("'%s'", string(v))
}
func (v StringValue) Eq(u Value) bool {
	if _, ok := u.(EmptyValue); ok {
		return true
	}

	if w, ok := u.(StringValue); ok {
		return bytes.Equal(v, w)
	}
	return false
}

type IntValue int64

func (v IntValue) String() string {
	return strconv.FormatInt(int64(v), 10)
}
func (v IntValue) Eq(u Value) bool {
	if _, ok := u.(EmptyValue); ok {
		return true
	}

	if w, ok := u.(IntValue); ok {
		return v == w
	}
	return false
}

type FloatValue float64

func (v FloatValue) String() string {
	return strconv.FormatFloat(float64(v), 'g', -1, 64)
}
func (v FloatValue) Eq(u Value) bool {
	if _, ok := u.(EmptyValue); ok {
		return true
	}

	if w, ok := u.(FloatValue); ok {
		return v == w
	}

	return false
}

type BoolValue bool

// interned booleans
const mgnTrue = BoolValue(true)
const mgnFalse = BoolValue(false)

func (v BoolValue) String() string {
	if v {
		return "true"
	}
	return "false"
}
func (v BoolValue) Eq(u Value) bool {
	if _, ok := u.(EmptyValue); ok {
		return true
	}

	if w, ok := u.(BoolValue); ok {
		return v == w
	}

	return false
}

type AtomValue string

func (v AtomValue) String() string {
	return ":" + string(v)
}
func (v AtomValue) Eq(u Value) bool {
	if _, ok := u.(EmptyValue); ok {
		return true
	}

	if w, ok := u.(AtomValue); ok {
		return v == w
	}

	return false
}

type ListValue []Value

func (v ListValue) String() string {
	valStrings := make([]string, len(v))
	for i, val := range v {
		valStrings[i] = val.String()
	}
	return "[" + strings.Join(valStrings, ", ") + "]"
}
func (v ListValue) Eq(u Value) bool {
	if _, ok := u.(EmptyValue); ok {
		return true
	}

	if w, ok := u.(ListValue); ok {
		if len(v) != len(w) {
			return false
		}

		for i, el := range v {
			if !el.Eq(w[i]) {
				return false
			}
		}
		return true
	}

	return false
}

type ObjectValue map[string]Value

func (v ObjectValue) String() string {
	// TODO: fix how this deals with circular references
	entryStrings := make([]string, len(v))
	i := 0
	for key, val := range v {
		entryStrings[i] = key + ": " + val.String()
		i++
	}
	return "{" + strings.Join(entryStrings, ", ") + "}"
}
func (v ObjectValue) Eq(u Value) bool {
	if _, ok := u.(EmptyValue); ok {
		return true
	}

	if w, ok := u.(ObjectValue); ok {
		if len(v) != len(w) {
			return false
		}

		for key, val := range v {
			if wVal, ok := w[key]; ok {
				if !val.Eq(wVal) {
					return false
				}
			} else {
				return false
			}
		}

		return true
	}

	return false
}

type FnValue struct {
	defn *fnNode
	scope
}

func (v FnValue) String() string {
	return v.defn.String()
}
func (v FnValue) Eq(u Value) bool {
	if w, ok := u.(FnValue); ok {
		return v.defn == w.defn
	}

	return false
}

type scope struct {
	parent *scope
	vars   map[string]Value
}

func (sc *scope) get(name string) (Value, error) {
	if v, ok := sc.vars[name]; ok {
		return v, nil
	}
	if sc.parent != nil {
		return sc.parent.get(name)
	}
	return nil, runtimeError{
		reason: fmt.Sprintf("%s is undefined", name),
	}
}

func (sc *scope) put(name string, v Value) {
	sc.vars[name] = v
}

func (sc *scope) update(name string, v Value) error {
	if _, ok := sc.vars[name]; ok {
		sc.vars[name] = v
		return nil
	}
	if sc.parent != nil {
		return sc.parent.update(name, v)
	}
	return runtimeError{
		reason: fmt.Sprintf("%s is undefined", name),
	}
}

type Context struct {
	// current working directory of this context, used for loading other
	// modules with relative paths / URLs
	Cwd string
	// path or descriptor of the file being run, used for error reporting
	SourcePath string
	// top level ("global") scope of this context
	scope
}

func NewContext(path, cwd string) Context {
	return Context{
		Cwd:        cwd,
		SourcePath: path,
		scope: scope{
			parent: nil,
			vars:   map[string]Value{},
		},
	}
}

func (c *Context) generateStackTrace() stackEntry {
	// TODO: actually write
	return stackEntry{}
}

type stackEntry struct {
	fnName      string
	parentStack *stackEntry
	pos
}

type vmError struct {
	reason string
}

func (e vmError) Error() string {
	return fmt.Sprintf("VM error: %s", e.reason)
}

type runtimeError struct {
	reason     string
	stackTrace stackEntry
}

func (e runtimeError) Error() string {
	// TODO: display stacktrace
	return fmt.Sprintf("Runtime error: %s", e.reason)
}

func (c *Context) Eval(programReader io.Reader) (Value, error) {
	program, err := io.ReadAll(programReader)
	if err != nil {
		return nil, err
	}

	tokenizer := newTokenizer(string(program))
	tokens := tokenizer.tokenize()

	parser := newParser(tokens)
	nodes, err := parser.parse()
	if err != nil {
		return nil, err
	}

	return c.evalProgram(nodes)
}

func (c *Context) evalProgram(nodes []astNode) (Value, error) {
	programBlock := blockNode{exprs: nodes}
	return c.evalExpr(programBlock, c.scope)
}

func (c *Context) evalExpr(node astNode, sc scope) (Value, error) {
	switch n := node.(type) {
	case emptyNode:
		return empty, nil
	case nullNode:
		return null, nil
	case stringNode:
		return StringValue([]byte(n.payload)), nil
	case numberNode:
		if n.isInteger {
			return IntValue(n.intPayload), nil
		}
		return FloatValue(n.floatPayload), nil
	case booleanNode:
		return BoolValue(n.payload), nil
	case atomNode:
		return AtomValue(n.payload), nil
	case listNode:
		var err error
		elems := make([]Value, len(n.elems))
		for i, elNode := range n.elems {
			elems[i], err = c.evalExpr(elNode, sc)
			if err != nil {
				return nil, err
			}
		}
		return ListValue(elems), nil
	case objectNode:
		obj := ObjectValue{}
		for _, entry := range n.entries {
			var keyString string

			if identKey, ok := entry.key.(identifierNode); ok {
				keyString = identKey.payload
			} else {
				key, err := c.evalExpr(entry.key, sc)
				if err != nil {
					return nil, err
				}
				switch typedKey := key.(type) {
				case StringValue:
					keyString = string(typedKey)
				case IntValue:
					keyString = typedKey.String()
				case FloatValue:
					keyString = typedKey.String()
				default:
					return nil, runtimeError{
						reason: fmt.Sprintf("Expected a string or number as object key, got %s", key.String()),
					}
				}
			}

			val, err := c.evalExpr(entry.val, sc)
			if err != nil {
				return nil, err
			}

			obj[keyString] = val
		}
		return obj, nil
	case fnNode:
		fn := FnValue{
			defn:  &n,
			scope: sc,
		}
		if fn.defn.name != "" {
			sc.put(fn.defn.name, fn)
		}
		return fn, nil
	case identifierNode:
		return sc.get(n.payload)
	case assignmentNode:
		assignedValue, err := c.evalExpr(n.right, sc)
		if err != nil {
			return nil, err
		}
		switch left := n.left.(type) {
		case identifierNode:
			if n.isLocal {
				sc.put(left.payload, assignedValue)
			} else {
				err := sc.update(left.payload, assignedValue)
				if err != nil {
					return nil, err
				}
			}
			return assignedValue, nil
		case listNode:
			// TODO: implement list destructuring assignment
			panic("list destructuring not implemented!")
		case objectNode:
			// TODO: implement object destructuring assignment
			panic("object destructuring not implemented!")
		case propertyAccessNode:
			// TODO: implement object property assignment
			panic("assign to property not implemented!")
		}
		panic(fmt.Sprintf("Illegal left-hand side of assignment in %s", n))
	case propertyAccessNode:
		left, err := c.evalExpr(n.left, sc)
		if err != nil {
			return nil, err
		}

		right, err := c.evalExpr(n.right, sc)
		if err != nil {
			return nil, err
		}

		switch target := left.(type) {
		case StringValue:
			byteIndex, ok := right.(IntValue)
			if !ok {
				return nil, runtimeError{
					reason: fmt.Sprintf("Cannot index into string with non-integer index %s", right),
				}
			}

			if byteIndex < 0 || int64(byteIndex) > int64(len(target)) {
				return null, nil
			}

			return StringValue([]byte{target[byteIndex]}), nil
		case ListValue:
			listIndex, ok := right.(IntValue)
			if !ok {
				return nil, runtimeError{
					reason: fmt.Sprintf("Cannot index into list with non-integer index %s", right),
				}
			}

			if listIndex < 0 || int64(listIndex) > int64(len(target)) {
				return null, nil
			}

			return target[listIndex], nil
		case ObjectValue:
			objKey := right.String()

			if val, ok := target[objKey]; ok {
				return val, nil
			}

			return null, nil
		}

		return nil, runtimeError{
			reason: fmt.Sprintf("Expected string, list, or object in left-hand side of property access, got %s", left.String()),
		}
	case unaryNode:
		// TODO: implement
		panic("unaryNode not implemented!")
	case binaryNode:
		// TODO: implement
		panic("binaryNode not implemented!")
	case fnCallNode:
		maybeFn, err := c.evalExpr(n.fn, sc)
		if err != nil {
			return nil, err
		}

		args := make([]Value, len(n.args))
		for i, argNode := range n.args {
			args[i], err = c.evalExpr(argNode, sc)
			if err != nil {
				return nil, err
			}
		}

		if fn, ok := maybeFn.(FnValue); ok {
			// TODO: implement restArgs
			args = args[:len(fn.defn.args)]
			fnScope := scope{
				parent: &fn.scope,
				vars:   map[string]Value{},
			}
			for i, argName := range fn.defn.args {
				fnScope.put(argName, args[i])
			}
			return c.evalExpr(fn.defn.body, fnScope)
		} else if fn, ok := maybeFn.(BuiltinFnValue); ok {
			return fn.fn(args)
		} else {
			return nil, runtimeError{
				reason: fmt.Sprintf("%s is not a function and cannot be called", maybeFn),
			}
		}
	case ifExprNode:
		cond, err := c.evalExpr(n.cond, sc)
		if err != nil {
			return nil, err
		}

		for _, branch := range n.branches {
			target, err := c.evalExpr(branch.target, sc)
			if err != nil {
				return nil, err
			}

			if cond.Eq(target) {
				return c.evalExpr(branch.body, sc)
			}
		}
		return null, nil
	case blockNode:
		var err error
		blockScope := scope{
			parent: &sc,
			vars:   map[string]Value{},
		}

		// empty block returns ? (null)
		var returnVal Value = null
		for _, expr := range n.exprs {
			returnVal, err = c.evalExpr(expr, blockScope)
			if err != nil {
				return nil, err
			}
		}
		return returnVal, nil
	}
	return null, nil
}
