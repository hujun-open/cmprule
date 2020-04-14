# Overview
cmprule is a golang module that compares a struct field's value against a rule defined in human friendly text.

An example use case: in case of test automation, there are result stats recorded in a Go struct, the stats need to be compared to expected values, but it is difficult to hard code all those compare logic; 

So this module allows user to provide a text rule, which specifies a struct field name, a compare operation and expected values, cmprule will parse the rule and does the comparison based on it. and it is generic enough that it works for all struct includes field types cmprule supports.

# Installation
cmprule is a module written purely in Go, doesn't require any 3rd party package. just include "import github.com/hujun-open/cmprule" in your code, and Go tool chain (that supports Go module) should take care of rest.

# Usage 
Following is an simple example:
```
package main

import (
	"log"
	"net"
	"time"

	"github.com/hujun-open/cmprule"
)

type TestStruct struct {
	Num1       int
	Float1     float32
	Str1       string
	TimeStamp1 time.Time
	Duration1  time.Duration
	IP1        net.IP
}

func main() {
	result1 := TestStruct{ //this is the result struct instance need to be compare against expect_results
		Num1:       -120,
		Float1:     12.5,
		Str1:       "test1",
		TimeStamp1: time.Now(),
		Duration1:  10 * time.Second,
		IP1:        net.ParseIP("1.1.1.1"),
	}
	expected_results := []string{
		`Num1:>:100`,                       //return true if Num1 value > 100
		`Float1:in:10 20`,                  //return true if Float1 value is in range of 10-20
		`Str1:contain:"test"`,              //return true if Str1 contains string "test"
		`TimeStamp1:>:1997/03/31T15:00:00`, //return true if TimeStamp1 is later than 1997/03/31 15:00:00
		`Duration1:>: 5s`,                  //return true if Duration1 is bigger than 5 seconds
		`IP1:within:1.1.1.0/24`,            //return true if IP1 within subnet 1.1.1.0/24
	}
	crule := cmprule.NewDefaultCMPRule() //create a CMPRule instance with default parsing functions
	for _, expected_result := range expected_results {
		err := crule.ParseRule(expected_result) //load a rule
		if err != nil {
			log.Fatal(err)
		}
		compare_result, err := crule.Compare(result1) //do the comparison
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("compare result for rule %v is %v", expected_result, compare_result)
	}
}


```
# Document
See documentation or comments in cmprule.go for detail usage



