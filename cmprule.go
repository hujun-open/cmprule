// Copyright 2020 Hu Jun. All rights reserved.
// This project is licensed under the terms of the MIT license.
// license that can be found in the LICENSE file.

/*
Package cmprule compare a field of a struct to a specified value,
based on a human-friendly rule in text.

An example use case is following:
	type ExampleStruct struct {
		Stat1 uint
		Stat2 float
	}
	example1:=ExampleStruct{Stat1:100,Stat2:20.5}

User could specify a rule:
	rule1:="Stat1 : >= : 50" //Stat1 must >= 50

Creats a CMPRule instance for above rule:
	cmp:=NewDefaultCMPRule()
	err := cmp.ParseRule(rule1)
	if err!=nil {
		//process error here
	}
	result,err:=cmp.Compare(example1)
	//result should be true, err should be nil

Supported Types

cmprule support following golang types of a struct field, along with corresponding pointer types:

	- int,int8,int16,int32,int64
	- uint,uint8,uint16,uint32,uint64
	- float32, float64
	- string
	- time.Time
	- time.Duration
	- net.IP
	- struct: this is specifically means nested struct


Default Rule Format

The default rule format is following:
	field_name : Op : Value
- field_name: the field name of the struct to compare

- Op: the compare operator

- Value: the vale to compare

field_name could have format as "aa.bb.cc" to support nested struct

Different type has different Op and Value format:

	- Numberic type: this includes all int/uint/float/time.Time/Time.Duration type in Golang
		- single value:
			- Op: ==,!=,>=,<=,>,<
			- Value: a single number
			- example: 'Stat1 : >= : 20'
		- Range value: return true if the field value is within/not within the range
			- Op: in, notin
			- Value: min,max number, sperated by space
			- example: 'Stat1 : in : 100 200'
		- A list of values: return true if the field value is one/none of the list
			- Op: is, not
			- Value: a list of numbers, sperated by space
			- example: 'Stat1 : is : 100 200 300 400'
		- Notes:
			- for time.Time, the string format is like const TIMEFMTSTR
			- for time.Duration, the string format is whatever supported by time.ParseDuration()
	- string:
		- a list of strings: return true if the field value is one/none of the list
			- Op: same, differ
			- Value: a list of double-quoted string, seperate by space
			- example: 'Result : same : "Passed without error" "Passed with error"
		-a list of strings: return true if the field value is contains/doesn't contain any string of the list
			- Op: contain, notcontain
			- Value: a list of double-quoted string, seperate by space
			- example: 'Lastlog : contain : "warning" "fail"
		- note: if the string in the value contain '"', use a backslash '\' to escape; like '\"'

	- net.IP:
		- a list of strings: return true if the field value is  within/not within to one/any prefix of the list
			- Op: within/notwithin
			- Value: a list of IP prefixes, seperate by space
			- example: 'MgmtAddr : within : 1.1.1.1/24 2001:dead::1/64'

Custom Rule Format

Optionally, the rule format could be customized by defining new parsing
function and pass it to CMPRule instances, by using CMPRule.SetxxxFunc(),
See corresponding function's doc for details.

*/
package cmprule

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// compare operators
const (
	opNumEq         = "=="
	opNumNotEq      = "!="
	opNumL          = ">"
	opNumLE         = ">="
	opNumS          = "<"
	opNumSE         = "<="
	opNumIN         = "in"
	opNumNotIN      = "notin"
	opNumIs         = "is"
	opNumNot        = "not"
	opStrSame       = "same"
	opStrDiffer     = "differ"
	opStrContain    = "contain"
	opStrNotContain = "notcontain"
	opIPWithin      = "within"
	opIPNotWithin   = "notwithin"
)

const (
	valueSingle = iota
	valueRange
	valueList
	valueInvalid
)

func detectType(op string) int {
	switch op {
	case opNumEq, opNumL, opNumLE, opNumNotEq, opNumS, opNumSE:
		return valueSingle
	case opNumIN, opNumNotIN:
		return valueRange
	case opNumIs, opNumNot:
		return valueList
	default:
		return valueInvalid
	}
}

const (
	prepareTypeNum = iota
	prepareTypeDuration
	prepareTypeTimestamp
	prepareTypeNotPrepared
)

// TimeFMTStr is the time format string used by default parse time function
const TimeFMTStr = "2006/01/02T15:04:05"

// ErrNilPoint is error for field in question is a nil pointer
var ErrNilPoint = errors.New("nil pointer")

// format: "fieldName:Op:Val"
func defaultDivideFunc(inputrule string) (string, string, string, error) {
	rule := strings.TrimSpace(inputrule)
	fields := strings.SplitN(rule, ":", 3)
	if len(fields) != 3 {
		return "", "", "", fmt.Errorf("invalid formatted rule, %v", rule)
	}
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2]), nil
}

//format: "min max"
func defaultParseRangeFunc(input string) (string, string, error) {
	fieldlist := strings.Fields(input)
	if len(fieldlist) != 2 {
		return "", "", fmt.Errorf("invalid range %v", input)
	}
	return fieldlist[0], fieldlist[1], nil
}

//format: "val-1 val-2 val-3 ..."
func defaultParseNumListFunc(input string) ([]string, error) {
	strlist := strings.Fields(input)
	if len(strlist) == 0 {
		return nil, fmt.Errorf("list is empty")
	}
	return strlist, nil
}

func defaultParseStrListFunc(input string) ([]string, error) {
	var p = regexp.MustCompile(`(?U)".*[^\\]"|""`)
	strlist := p.FindAllString(input, -1)
	if len(strlist) == 0 {
		return nil, fmt.Errorf("list is empty")
	}
	var r []string
	for _, s := range strlist {
		r = append(r, strings.ReplaceAll(s[1:len(s)-1], `\"`, `"`))
	}
	return r, nil
}

func defaultParseNumInt64Func(numstr string) (int64, error) {
	return strconv.ParseInt(numstr, 0, 64)
}

func defaultParseDurationInt64Func(durationstr string) (int64, error) {
	d, err := time.ParseDuration(durationstr)
	if err != nil {
		return 0, err
	}
	return d.Nanoseconds(), nil
}

func defaultParseIPNetListFunc(listval string) ([]*net.IPNet, error) {
	prefixStrList := strings.Fields(listval)
	var r []*net.IPNet
	for _, prefixStr := range prefixStrList {
		_, prefix, err := net.ParseCIDR(prefixStr)
		if err != nil {
			return nil, err
		}
		r = append(r, prefix)
	}
	return r, nil

}

func defaultParseTimeInt64Func(timestr string) (int64, error) {
	t, err := time.Parse(TimeFMTStr, timestr)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// use "." as seperator, like "aaa.bbb.ccc"
func defaultParseNestedStructFunc(fieldName string) []string {
	return strings.Split(fieldName, ".")
}

// return a struct field based on field_name_list, which is hierchical name list
func getStructField(inputStruct interface{}, fieldNameList []string) (interface{}, error) {
	currentStruct := inputStruct
	listLen := len(fieldNameList)
	var currentType reflect.Type
	var currentVal reflect.Value
	var i int
	var fname string
	for i, fname = range fieldNameList {
		currentType = reflect.TypeOf(currentStruct)
		currentVal = reflect.ValueOf(currentStruct)
		//if the field is a pointer, return the interface{} it points to
		if currentType.Kind() == reflect.Ptr {
			if currentVal.IsZero() {
				return nil, fmt.Errorf("%v is %w", currentType, ErrNilPoint)
			}
			currentStruct = reflect.Indirect(currentVal).Interface()
			currentType = reflect.TypeOf(currentStruct)
			currentVal = reflect.ValueOf(currentStruct)
		}

		if currentType.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%v is not a struct", fieldNameList[i-1])
		}

		if _, ok := currentType.FieldByName(fname); !ok {
			return nil, fmt.Errorf("field %v doesn't exist in %v", fname, currentType.String())
		}
		if currentType.Kind() != reflect.Struct && i != listLen-1 {
			return nil, fmt.Errorf("%v is not a struct", currentType.String())
		}
		currentStruct = currentVal.FieldByName(fname).Interface()
	}
	if reflect.TypeOf(currentStruct).Kind() == reflect.Ptr {
		if reflect.ValueOf(currentStruct).IsZero() {
			return nil, fmt.Errorf("%v is %w", reflect.TypeOf(currentStruct), ErrNilPoint)
		}
		return reflect.Indirect(reflect.ValueOf(currentStruct)).Interface(), nil
	}
	return currentStruct, nil
}

// CMPRule represents a single compare rule
type CMPRule struct {
	ruleFieldName          string
	ruleOp                 string
	ruleVal                string
	divideRuleFunc         func(rule string) (string, string, string, error)
	parseRangeFunc         func(rangeval string) (string, string, error)
	parseNumListFunc       func(listval string) ([]string, error)
	parseIPNetListFunc     func(listval string) ([]*net.IPNet, error)
	parseStrListFunc       func(listval string) ([]string, error)
	parseNumInt64Func      func(numstr string) (int64, error)
	parseDurationInt64Func func(durationstr string) (int64, error)
	parseTimeInt64Func     func(timestr string) (int64, error)
	parseFieldNamFunc      func(field_name string) []string
	prepareInt64Type       int
	numMinStr              string
	numMaxStr              string
	numListStr             []string
	int64Single            int64
	int64Min               int64
	int64Max               int64
	int64List              []int64
	strList                []string
	ipNetList              []*net.IPNet
	fieldNameList          []string
}

// NewDefaultCMPRule Returns a CMPRule instance with default parse functions
func NewDefaultCMPRule() *CMPRule {
	r := new(CMPRule)
	r.divideRuleFunc = defaultDivideFunc
	r.parseNumListFunc = defaultParseNumListFunc
	r.parseRangeFunc = defaultParseRangeFunc
	r.parseStrListFunc = defaultParseStrListFunc
	r.parseNumInt64Func = defaultParseNumInt64Func
	r.parseDurationInt64Func = defaultParseDurationInt64Func
	r.parseTimeInt64Func = defaultParseTimeInt64Func
	r.parseIPNetListFunc = defaultParseIPNetListFunc
	r.parseFieldNamFunc = defaultParseNestedStructFunc
	r.prepareInt64Type = prepareTypeNotPrepared
	return r
}

// ParseRule Parses a string to get a rule, see package doc for the default format of the rawrule string
func (cmprule *CMPRule) ParseRule(rawrule string) (err error) {
	cmprule.ruleFieldName, cmprule.ruleOp, cmprule.ruleVal, err = cmprule.divideRuleFunc(rawrule)
	switch cmprule.ruleOp {
	case opNumIN, opNumNotIN:
		cmprule.numMinStr, cmprule.numMaxStr, err = cmprule.parseRangeFunc(cmprule.ruleVal)
	case opNumIs, opNumNot:
		cmprule.numListStr, err = cmprule.parseNumListFunc(cmprule.ruleVal)
	case opStrContain, opStrDiffer, opStrNotContain, opStrSame:
		cmprule.strList, err = cmprule.parseStrListFunc(cmprule.ruleVal)
	case opIPWithin, opIPNotWithin:
		cmprule.ipNetList, err = cmprule.parseIPNetListFunc(cmprule.ruleVal)
	}
	cmprule.prepareInt64Type = prepareTypeNotPrepared
	cmprule.fieldNameList = cmprule.parseFieldNamFunc(cmprule.ruleFieldName)
	return
}

func (cmprule *CMPRule) prepareInt64(f func(string) (int64, error)) (err error) {
	optype := detectType(cmprule.ruleOp)
	switch optype {
	case valueSingle:
		cmprule.int64Single, err = f(cmprule.ruleVal)
		return
	case valueRange:
		cmprule.int64Min, err = f(cmprule.numMinStr)
		if err != nil {
			return
		}
		cmprule.int64Max, err = f(cmprule.numMaxStr)
		if err != nil {
			return
		}
		if cmprule.int64Max < cmprule.int64Min {
			err = fmt.Errorf("invalid range value, max value is smaller than min value")
		}
		return
	case valueList:
		cmprule.int64List = []int64{}
		for _, str := range cmprule.numListStr {
			var v int64
			v, err = f(str)
			if err != nil {
				return
			}
			cmprule.int64List = append(cmprule.int64List, v)
		}
		return
	default:
		return fmt.Errorf("invalid op for int64,%v", cmprule.ruleOp)
	}
}

func (cmprule *CMPRule) compareElement(element interface{}) (bool, error) {
	etype := reflect.TypeOf(element)
	fieldVal := reflect.ValueOf(element)
	switch etype.String() {
	case "int", "int8", "int16", "int32", "int64":
		if cmprule.prepareInt64Type != prepareTypeNum {
			err := cmprule.prepareInt64(cmprule.parseNumInt64Func)
			if err != nil {
				return false, err
			}
			cmprule.prepareInt64Type = prepareTypeNum
		}
		return cmprule.compareNumberic(fieldVal.Int())
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return cmprule.compareNumberic(fieldVal.Uint())
	case "float32", "float64":
		return cmprule.compareNumberic(fieldVal.Float())
	case "string":
		return cmprule.compareString(fieldVal.String())
	case "time.Duration":
		if cmprule.prepareInt64Type != prepareTypeDuration {
			err := cmprule.prepareInt64(cmprule.parseDurationInt64Func)
			if err != nil {
				return false, err
			}
			cmprule.prepareInt64Type = prepareTypeDuration
		}
		return cmprule.compareNumberic(fieldVal.Interface().(time.Duration).Nanoseconds())
	case "time.Time":
		if cmprule.prepareInt64Type != prepareTypeTimestamp {
			err := cmprule.prepareInt64(cmprule.parseTimeInt64Func)
			if err != nil {
				return false, err
			}
			cmprule.prepareInt64Type = prepareTypeTimestamp
		}
		return cmprule.compareNumberic(fieldVal.Interface().(time.Time).Unix())
	case "net.IP":
		return cmprule.compareIP(fieldVal.Interface().(net.IP))
	default:
		return false, fmt.Errorf("field %v has unsupported type %v", cmprule.ruleFieldName, etype.String())
	}
}

// Compare to input, which must be a struct, based on parsed rules
// return true/false if comparison is done successfully
// return a non-nil error if fail to do the comparison
func (cmprule *CMPRule) Compare(input interface{}) (bool, error) {
	fieldInt, err := getStructField(input, cmprule.fieldNameList)
	if err != nil {
		return false, err
	}
	return cmprule.compareElement(fieldInt)
}

func (cmprule *CMPRule) compareIP(inputip net.IP) (bool, error) {
	for _, prefix := range cmprule.ipNetList {
		if prefix.Contains(inputip) {
			return true, nil
		}
	}
	return false, nil
}

func (cmprule *CMPRule) compareString(input string) (bool, error) {
	switch cmprule.ruleOp {
	case opStrSame:
		for _, val := range cmprule.strList {
			if val == input {
				return true, nil
			}
		}
		return false, nil
	case opStrDiffer:
		found := false
		for _, val := range cmprule.strList {
			if val == input {
				found = true
				break
			}
		}
		return !found, nil
	case opStrContain:
		for _, val := range cmprule.strList {
			if strings.Contains(input, val) {
				return true, nil
			}
		}
		return false, nil
	case opStrNotContain:
		found := false
		for _, val := range cmprule.strList {
			if strings.Contains(input, val) {
				found = true
				break
			}
		}
		return !found, nil
	default:
		return false, fmt.Errorf("invalid op %v for string", cmprule.ruleOp)
	}

}

// ClearPreparedInt64Value Clear the previous pre-parsed int64 values, this is only needed when compare a new type of struct with a already parsed rule
// e.g. this is not needed, if you use same rule to compare different instances of same type of struct
func (cmprule *CMPRule) ClearPreparedInt64Value() {
	cmprule.prepareInt64Type = prepareTypeNotPrepared
}

//input could only be int64,uint64 or float64
func (cmprule *CMPRule) compareNumberic(input interface{}) (bool, error) {
	inputKind := reflect.TypeOf(input).Kind()
	switch inputKind {
	case reflect.Int64:
		inputval := input.(int64)
		vtype := detectType(cmprule.ruleOp)
		switch vtype {
		case valueSingle:
			switch cmprule.ruleOp {
			case "==":
				return cmprule.int64Single == inputval, nil
			case "!=":
				return cmprule.int64Single != inputval, nil
			case ">=":
				return inputval >= cmprule.int64Single, nil
			case "<=":
				return inputval <= cmprule.int64Single, nil
			case ">":
				return inputval > cmprule.int64Single, nil
			case "<":
				return inputval < cmprule.int64Single, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		case valueRange:
			switch cmprule.ruleOp {
			case "in":
				return inputval >= cmprule.int64Min && inputval <= cmprule.int64Max, nil
			case "notin":
				return !(inputval >= cmprule.int64Min && inputval <= cmprule.int64Max), nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		case valueList:
			found := false
			for _, v := range cmprule.int64List {
				if inputval == v {
					found = true
					break
				}
			}
			switch cmprule.ruleOp {
			case "is":
				return found, nil
			case "not":
				return !found, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v, value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		default:
			return false, fmt.Errorf("invalid op and/or value: %v %v", cmprule.ruleOp, cmprule.ruleVal)
		}
	case reflect.Uint64:
		inputval := input.(uint64)
		vtype := detectType(cmprule.ruleOp)
		switch vtype {
		case valueSingle:
			singleVal, err := strconv.ParseUint(cmprule.ruleVal, 0, 64)
			if err != nil {
				return false, fmt.Errorf("can't parse %v into int", cmprule.ruleVal)
			}
			switch cmprule.ruleOp {
			case "==":
				return singleVal == inputval, nil
			case "!=":
				return singleVal != inputval, nil
			case ">=":
				return inputval >= singleVal, nil
			case "<=":
				return inputval <= singleVal, nil
			case ">":
				return inputval > singleVal, nil
			case "<":
				return inputval < singleVal, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		case valueRange:
			min, err := strconv.ParseUint(cmprule.numMinStr, 0, 64)
			if err != nil {
				return false, fmt.Errorf("invalid range value %v", cmprule.numMinStr)
			}
			max, err := strconv.ParseUint(cmprule.numMaxStr, 0, 64)
			if err != nil {
				return false, fmt.Errorf("invalid range value %v", cmprule.numMaxStr)
			}
			if min > max {
				return false, fmt.Errorf("invalid range value, min>max %v", cmprule.ruleVal)
			}
			switch cmprule.ruleOp {
			case "in":
				return inputval >= min && inputval <= max, nil
			case "notin":
				return !(inputval >= min && inputval <= max), nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		case valueList:
			var vallist []uint64
			for _, s := range cmprule.numListStr {
				v, err := strconv.ParseUint(s, 0, 64)
				if err != nil {
					return false, fmt.Errorf("%v is not a valid int value", s)
				}
				vallist = append(vallist, v)
			}
			found := false
			for _, v := range vallist {
				if inputval == v {
					found = true
					break
				}
			}
			switch cmprule.ruleOp {
			case "is":
				return found, nil
			case "not":
				return !found, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v, value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		default:
			return false, fmt.Errorf("invalid op and/or value: %v %v", cmprule.ruleOp, cmprule.ruleVal)

		}
	case reflect.Float64:
		inputval := input.(float64)
		vtype := detectType(cmprule.ruleOp)
		switch vtype {
		case valueSingle:
			singleVal, err := strconv.ParseFloat(cmprule.ruleVal, 64)
			if err != nil {
				return false, fmt.Errorf("can't parse %v into int", cmprule.ruleVal)
			}
			switch cmprule.ruleOp {
			case "==":
				return singleVal == inputval, nil
			case "!=":
				return singleVal != inputval, nil
			case ">=":
				return inputval >= singleVal, nil
			case "<=":
				return inputval <= singleVal, nil
			case ">":
				return inputval > singleVal, nil
			case "<":
				return inputval < singleVal, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		case valueRange:
			min, err := strconv.ParseFloat(cmprule.numMinStr, 64)
			if err != nil {
				return false, fmt.Errorf("invalid range value %v", cmprule.numMinStr)
			}
			max, err := strconv.ParseFloat(cmprule.numMaxStr, 64)
			if err != nil {
				return false, fmt.Errorf("invalid range value %v", cmprule.numMaxStr)
			}
			if min > max {
				return false, fmt.Errorf("invalid range value, min>max %v", cmprule.ruleVal)
			}
			switch cmprule.ruleOp {
			case "in":
				return inputval >= min && inputval <= max, nil
			case "notin":
				return !(inputval >= min && inputval <= max), nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		case valueList:
			var vallist []float64
			for _, s := range cmprule.strList {
				v, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return false, fmt.Errorf("%v is not a valid int value", s)
				}
				vallist = append(vallist, v)
			}
			found := false
			for _, v := range vallist {
				if inputval == v {
					found = true
					break
				}
			}
			switch cmprule.ruleOp {
			case "is":
				return found, nil
			case "not":
				return !found, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v, value %v", cmprule.ruleOp, inputKind, cmprule.ruleVal)
			}
		default:
			return false, fmt.Errorf("invalid op and/or value: %v %v", cmprule.ruleOp, cmprule.ruleVal)

		}
	default:
		return false, fmt.Errorf("unsupported type:%v", inputKind)
	}
}

// SetDivideRuleFunc set f as function to divide rawrule into 3 strings as struct field_name, operator, values.
// this is used by all types.
// default function use ":" as seperator, 1st field as filed_name, 2nd as operator, rest of stirng become values
func (cmprule *CMPRule) SetDivideRuleFunc(f func(rule string) (string, string, string, error)) {
	cmprule.divideRuleFunc = f
}

// SetParseRangeFunc set f as function to parse a string that represents a range into two string contains min, max value.
// this is used by all types support OP_NUM_IN and OP_NUM_NOTIN.
// default function uses spaces as sperator
func (cmprule *CMPRule) SetParseRangeFunc(f func(rangeval string) (string, string, error)) {
	cmprule.parseRangeFunc = f
}

// SetParseNumListFunc set f as function to parse a string that represents a number list into a slice of string, each contains a number.
// this is used by all types support OP_NUM_IS and OP_NUM_NOT.
// default function uses spaces as sperator.
func (cmprule *CMPRule) SetParseNumListFunc(f func(listval string) ([]string, error)) {
	cmprule.parseNumListFunc = f
}

// SetParseIPNetListFunc set f as function to parse a string that represents a IP prefixes into a slice of *net.IPNet.
// this is used only by type net.IP.
// default function uses spaces as sperator, and uses net.ParseCIDR
func (cmprule *CMPRule) SetParseIPNetListFunc(f func(listval string) ([]*net.IPNet, error)) {
	cmprule.parseIPNetListFunc = f
}

// SetParseStrListFunc set f as function to parse a string that represents a list of string into a slice of string.
// this is used only by type string.
// default function uses space as seperator.
func (cmprule *CMPRule) SetParseStrListFunc(f func(listval string) ([]string, error)) {
	cmprule.parseStrListFunc = f
}

// SetParseNumInt64Func set f as function to parse a string that represents a number into int64
// this is used by type int,int8,int16,int32,int64.
// default function uses strconv.ParseInt(numstr, 0, 64).
func (cmprule *CMPRule) SetParseNumInt64Func(f func(numstr string) (int64, error)) {
	cmprule.parseNumInt64Func = f
}

// SetparseDurationInt64Func set f as function to parse a string that represents time.Duration into int64.
// this is used only by type time.Duration.
// default function uses time.ParseDuration
func (cmprule *CMPRule) SetparseDurationInt64Func(f func(durationstr string) (int64, error)) {
	cmprule.parseDurationInt64Func = f
}

// SetparseTimeInt64Func set f as function to parse a string that represents time.Time into int64.
// this is used only by type time.Time.
// default function uses time.Parse, with format string as const TIMEFMTSTR
func (cmprule *CMPRule) SetparseTimeInt64Func(f func(timestr string) (int64, error)) {
	cmprule.parseTimeInt64Func = f
}

// SetParseFieldNameFunc set f as function to parse field_name string into a list field name,
// each represents a field name in the nested struct
// default function use "." as seperator like "aa.bb.cc"
func (cmprule *CMPRule) SetParseFieldNameFunc(f func(field_name string) []string) {
	cmprule.parseFieldNamFunc = f
}
