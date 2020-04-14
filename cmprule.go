// Copyright 2020 Hu Jun. All rights reserved.
// This project is licensed under the terms of the MIT license.

/*Package cmprule compare a field of a struct to a specified value, based on a human-friendly rule in text.

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

cmprule support following golang types of a struct field:

	- int,int8,int16,int32,int64
	- uint,uint8,uint16,uint32,uint64
	- float32, float64
	- string
	- time.Time
	- time.Duration
	- net.IP


Default Rule Format

The default rule format is following:
	field_name : Op : Value
- field_name: the field name of the struct to compare

- Op: the compare operator

- Value: the vale to compare

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

Optionally, the rule format could be customized by defining new parsing function and pass it to CMPRule instances,
by using CMPRule.SetxxxFunc(), see corresponding function's doc for details

*/
package cmprule

import (
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	OP_NUM_EQ         = "=="
	OP_NUM_NOTEQ      = "!="
	OP_NUM_L          = ">"
	OP_NUM_LE         = ">="
	OP_NUM_S          = "<"
	OP_NUM_SE         = "<="
	OP_NUM_IN         = "in"
	OP_NUM_NOTIN      = "notin"
	OP_NUM_IS         = "is"
	OP_NUM_NOT        = "not"
	OP_STR_SAME       = "same"
	OP_STR_DIFFER     = "differ"
	OP_STR_CONTAIN    = "contain"
	OP_STR_NOTCONTAIN = "notcontain"
	OP_IP_WIHTIN      = "within"
	OP_IP_NOTWIHTIN   = "notwithin"
)

const (
	VALUE_SINGLE = iota
	VALUE_RANGE
	VALUE_LIST
	VALUE_INVALID
)

func detectType(op string) int {
	switch op {
	case OP_NUM_EQ, OP_NUM_L, OP_NUM_LE, OP_NUM_NOTEQ, OP_NUM_S, OP_NUM_SE:
		return VALUE_SINGLE
	case OP_NUM_IN, OP_NUM_NOTIN:
		return VALUE_RANGE
	case OP_NUM_IS, OP_NUM_NOT:
		return VALUE_LIST
	default:
		return VALUE_INVALID
	}
}

const (
	PREPARE_TYPE_NUM = iota
	PREPARE_TYPE_DURATION
	PREPARE_TYPE_TIMESTAMP
	PREPARE_TYPE_NOTPREPARED
)

const TIMEFMTSTR = "2006/01/02T15:04:05"

//format: "fieldName:Op:Val"
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
	prefix_str_list := strings.Fields(listval)
	var r []*net.IPNet
	for _, prefix_str := range prefix_str_list {
		_, prefix, err := net.ParseCIDR(prefix_str)
		if err != nil {
			return nil, err
		}
		r = append(r, prefix)
	}
	return r, nil

}

func defaultParseTimeInt64Func(timestr string) (int64, error) {
	t, err := time.Parse(TIMEFMTSTR, timestr)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

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
}

//Returns a CMPRule instance with default parse functions
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
	r.prepareInt64Type = PREPARE_TYPE_NOTPREPARED
	return r
}

//Parse a rule, see package doc for the default format of the rawrule string
func (self *CMPRule) ParseRule(rawrule string) (err error) {
	self.ruleFieldName, self.ruleOp, self.ruleVal, err = self.divideRuleFunc(rawrule)
	switch self.ruleOp {
	case OP_NUM_IN, OP_NUM_NOTIN:
		self.numMinStr, self.numMaxStr, err = self.parseRangeFunc(self.ruleVal)
	case OP_NUM_IS, OP_NUM_NOT:
		self.numListStr, err = self.parseNumListFunc(self.ruleVal)
	case OP_STR_CONTAIN, OP_STR_DIFFER, OP_STR_NOTCONTAIN, OP_STR_SAME:
		self.strList, err = self.parseStrListFunc(self.ruleVal)
	case OP_IP_WIHTIN, OP_IP_NOTWIHTIN:
		self.ipNetList, err = self.parseIPNetListFunc(self.ruleVal)
	}
	self.prepareInt64Type = PREPARE_TYPE_NOTPREPARED
	return
}

func (self *CMPRule) prepareInt64(f func(string) (int64, error)) (err error) {
	optype := detectType(self.ruleOp)
	switch optype {
	case VALUE_SINGLE:
		self.int64Single, err = f(self.ruleVal)
		return
	case VALUE_RANGE:
		self.int64Min, err = f(self.numMinStr)
		if err != nil {
			return
		}
		self.int64Max, err = f(self.numMaxStr)
		if err != nil {
			return
		}
		if self.int64Max < self.int64Min {
			err = fmt.Errorf("invalid range value, max value is smaller than min value")
		}
		return
	case VALUE_LIST:
		self.int64List = []int64{}
		for _, str := range self.numListStr {
			var v int64
			v, err = f(str)
			if err != nil {
				return
			}
			self.int64List = append(self.int64List, v)
		}
		return
	default:
		return fmt.Errorf("invalid op for int64,%v", self.ruleOp)
	}
}

//Compare to input, which must be a struct, based on parsed rules
//return true/false if comparison is done successfully
//return a non-nil error if fail to do the comparison
func (self *CMPRule) Compare(input interface{}) (bool, error) {
	input_type := reflect.TypeOf(input)
	if input_type.Kind() != reflect.Struct {
		return false, fmt.Errorf("input is not a struct")
	}
	input_value := reflect.ValueOf(input)
	sf, exists := input_type.FieldByName(self.ruleFieldName)
	if !exists {
		return false, fmt.Errorf("field %v doesn't exists in the input struct", self.ruleFieldName)
	}
	field_value := input_value.FieldByName(self.ruleFieldName)
	switch sf.Type.String() {
	case "int", "int8", "int16", "int32", "int64":
		if self.prepareInt64Type != PREPARE_TYPE_NUM {
			err := self.prepareInt64(self.parseNumInt64Func)
			if err != nil {
				return false, err
			}
			self.prepareInt64Type = PREPARE_TYPE_NUM
		}
		return self.compareNumberic(field_value.Int())
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return self.compareNumberic(field_value.Uint())
	case "float32", "float64":
		return self.compareNumberic(field_value.Float())
	case "string":
		return self.compareString(field_value.String())
	case "time.Duration":
		if self.prepareInt64Type != PREPARE_TYPE_DURATION {
			err := self.prepareInt64(self.parseDurationInt64Func)
			if err != nil {
				return false, err
			}
			self.prepareInt64Type = PREPARE_TYPE_DURATION
		}
		return self.compareNumberic(field_value.Interface().(time.Duration).Nanoseconds())
	case "time.Time":
		if self.prepareInt64Type != PREPARE_TYPE_TIMESTAMP {
			err := self.prepareInt64(self.parseTimeInt64Func)
			if err != nil {
				return false, err
			}
			self.prepareInt64Type = PREPARE_TYPE_TIMESTAMP
		}
		return self.compareNumberic(field_value.Interface().(time.Time).Unix())
	case "net.IP":
		return self.compareIP(field_value.Interface().(net.IP))
	default:
		return false, fmt.Errorf("field %v has unsupported type %v", self.ruleFieldName, sf.Type.String())
	}
}

func (self *CMPRule) compareIP(input_ip net.IP) (bool, error) {
	for _, prefix := range self.ipNetList {
		if prefix.Contains(input_ip) {
			return true, nil
		}
	}
	return false, nil
}

func (self *CMPRule) compareString(input string) (bool, error) {
	switch self.ruleOp {
	case OP_STR_SAME:
		for _, val := range self.strList {
			if val == input {
				return true, nil
			}
		}
		return false, nil
	case OP_STR_DIFFER:
		found := false
		for _, val := range self.strList {
			if val == input {
				found = true
				break
			}
		}
		return !found, nil
	case OP_STR_CONTAIN:
		for _, val := range self.strList {
			if strings.Contains(input, val) {
				return true, nil
			}
		}
		return false, nil
	case OP_STR_NOTCONTAIN:
		found := false
		for _, val := range self.strList {
			if strings.Contains(input, val) {
				found = true
				break
			}
		}
		return !found, nil
	default:
		return false, fmt.Errorf("invalid op %v for string", self.ruleOp)
	}

}

//Clear the previous pre-parsed int64 values, this is only needed when compare a new type of struct with a already parsed rule
//e.g. this is not needed, if you use same rule to compare different instances of same type of struct
func (self *CMPRule) ClearPreparedInt64Value() {
	self.prepareInt64Type = PREPARE_TYPE_NOTPREPARED
}

//input could only be int64,uint64 or float64
func (self *CMPRule) compareNumberic(input interface{}) (bool, error) {
	input_kind := reflect.TypeOf(input).Kind()
	switch input_kind {
	case reflect.Int64:
		inputval := input.(int64)
		vtype := detectType(self.ruleOp)
		switch vtype {
		case VALUE_SINGLE:
			switch self.ruleOp {
			case "==":
				return self.int64Single == inputval, nil
			case "!=":
				return self.int64Single != inputval, nil
			case ">=":
				return inputval >= self.int64Single, nil
			case "<=":
				return inputval <= self.int64Single, nil
			case ">":
				return inputval > self.int64Single, nil
			case "<":
				return inputval < self.int64Single, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		case VALUE_RANGE:
			switch self.ruleOp {
			case "in":
				return inputval >= self.int64Min && inputval <= self.int64Max, nil
			case "notin":
				return !(inputval >= self.int64Min && inputval <= self.int64Max), nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		case VALUE_LIST:
			found := false
			for _, v := range self.int64List {
				if inputval == v {
					found = true
					break
				}
			}
			switch self.ruleOp {
			case "is":
				return found, nil
			case "not":
				return !found, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v, value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		default:
			return false, fmt.Errorf("invalid op and/or value: %v %v", self.ruleOp, self.ruleVal)
		}
	case reflect.Uint64:
		inputval := input.(uint64)
		vtype := detectType(self.ruleOp)
		switch vtype {
		case VALUE_SINGLE:
			single_val, err := strconv.ParseUint(self.ruleVal, 0, 64)
			if err != nil {
				return false, fmt.Errorf("can't parse %v into int", self.ruleVal)
			}
			switch self.ruleOp {
			case "==":
				return single_val == inputval, nil
			case "!=":
				return single_val != inputval, nil
			case ">=":
				return inputval >= single_val, nil
			case "<=":
				return inputval <= single_val, nil
			case ">":
				return inputval > single_val, nil
			case "<":
				return inputval < single_val, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		case VALUE_RANGE:
			min, err := strconv.ParseUint(self.numMinStr, 0, 64)
			if err != nil {
				return false, fmt.Errorf("invalid range value %v", self.numMinStr)
			}
			max, err := strconv.ParseUint(self.numMaxStr, 0, 64)
			if err != nil {
				return false, fmt.Errorf("invalid range value %v", self.numMaxStr)
			}
			if min > max {
				return false, fmt.Errorf("invalid range value, min>max %v", self.ruleVal)
			}
			switch self.ruleOp {
			case "in":
				return inputval >= min && inputval <= max, nil
			case "notin":
				return !(inputval >= min && inputval <= max), nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		case VALUE_LIST:
			var vallist []uint64
			for _, s := range self.numListStr {
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
			switch self.ruleOp {
			case "is":
				return found, nil
			case "not":
				return !found, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v, value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		default:
			return false, fmt.Errorf("invalid op and/or value: %v %v", self.ruleOp, self.ruleVal)

		}
	case reflect.Float64:
		inputval := input.(float64)
		vtype := detectType(self.ruleOp)
		switch vtype {
		case VALUE_SINGLE:
			single_val, err := strconv.ParseFloat(self.ruleVal, 64)
			if err != nil {
				return false, fmt.Errorf("can't parse %v into int", self.ruleVal)
			}
			switch self.ruleOp {
			case "==":
				return single_val == inputval, nil
			case "!=":
				return single_val != inputval, nil
			case ">=":
				return inputval >= single_val, nil
			case "<=":
				return inputval <= single_val, nil
			case ">":
				return inputval > single_val, nil
			case "<":
				return inputval < single_val, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		case VALUE_RANGE:
			min, err := strconv.ParseFloat(self.numMinStr, 64)
			if err != nil {
				return false, fmt.Errorf("invalid range value %v", self.numMinStr)
			}
			max, err := strconv.ParseFloat(self.numMaxStr, 64)
			if err != nil {
				return false, fmt.Errorf("invalid range value %v", self.numMaxStr)
			}
			if min > max {
				return false, fmt.Errorf("invalid range value, min>max %v", self.ruleVal)
			}
			switch self.ruleOp {
			case "in":
				return inputval >= min && inputval <= max, nil
			case "notin":
				return !(inputval >= min && inputval <= max), nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		case VALUE_LIST:
			var vallist []float64
			for _, s := range self.strList {
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
			switch self.ruleOp {
			case "is":
				return found, nil
			case "not":
				return !found, nil
			default:
				return false, fmt.Errorf("invalid op %v for type %v, value %v", self.ruleOp, input_kind, self.ruleVal)
			}
		default:
			return false, fmt.Errorf("invalid op and/or value: %v %v", self.ruleOp, self.ruleVal)

		}
	default:
		return false, fmt.Errorf("unsupported type:%v", input_kind)
	}
}

//Set f as function to divide rawrule into 3 strings as struct field_name, operator, values.
//this is used by all types.
//default function use ":" as seperator, 1st field as filed_name, 2nd as operator, rest of stirng become values
func (self *CMPRule) SetDivideRuleFunc(f func(rule string) (string, string, string, error)) {
	self.divideRuleFunc = f
}

//Set f as function to parse a string that represents a range into two string contains min, max value.
//this is used by all types support OP_NUM_IN and OP_NUM_NOTIN.
//default function uses spaces as sperator
func (self *CMPRule) SetParseRangeFunc(f func(rangeval string) (string, string, error)) {
	self.parseRangeFunc = f
}

//Set f as function to parse a string that represents a number list into a slice of string, each contains a number.
//this is used by all types support OP_NUM_IS and OP_NUM_NOT.
//default function uses spaces as sperator.
func (self *CMPRule) SetParseNumListFunc(f func(listval string) ([]string, error)) {
	self.parseNumListFunc = f
}

//Set f as function to parse a string that represents a IP prefixes into a slice of *net.IPNet.
//this is used only by type net.IP.
//default function uses spaces as sperator, and uses net.ParseCIDR
func (self *CMPRule) SetParseIPNetListFunc(f func(listval string) ([]*net.IPNet, error)) {
	self.parseIPNetListFunc = f
}

//Set f as function to parse a string that represents a list of string into a slice of string.
//this is used only by type string.
//default function uses space as seperator.
func (self *CMPRule) SetParseStrListFunc(f func(listval string) ([]string, error)) {
	self.parseStrListFunc = f
}

//Set f as function to parse a string that represents a number into int64
//this is used by type int,int8,int16,int32,int64.
//default function uses strconv.ParseInt(numstr, 0, 64).
func (self *CMPRule) SetParseNumInt64Func(f func(numstr string) (int64, error)) {
	self.parseNumInt64Func = f
}

//Set f as function to parse a string that represents time.Duration into int64.
//this is used only by type time.Duration.
//default function uses time.ParseDuration
func (self *CMPRule) SetparseDurationInt64Func(f func(durationstr string) (int64, error)) {
	self.parseDurationInt64Func = f
}

//Set f as function to parse a string that represents time.Time into int64.
//this is used only by type time.Time.
//default function uses time.Parse, with format string as const TIMEFMTSTR
func (self *CMPRule) SetparseTimeInt64Func(f func(timestr string) (int64, error)) {
	self.parseTimeInt64Func = f
}
