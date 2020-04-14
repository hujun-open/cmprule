// cmprule_test
package cmprule

import (
	"net"
	"testing"
	"time"
)

type testStruct struct {
	Num1      int
	Num_uint1 uint
	Float1    float32
	Str1      string
	Str2      string
	Stamp1    time.Time
	Duration1 time.Duration
	IP1       net.IP
	IP2       net.IP
}

var test_struct testStruct = testStruct{
	Num1: -120, Num_uint1: 120, Float1: 12.5, Str1: "test1", Str2: `"inside"outside`,
	Duration1: 10 * time.Second,
	IP1:       net.ParseIP("1.1.1.1"),
	IP2:       net.ParseIP("2001:dead::1"),
}
var test_list = []struct {
	in         string
	out_bool   bool
	expect_err bool
}{
	//int
	{"Num1:==:-120", true, false},
	{" Num1 :  ==: -120 ", true, false},
	{" Num1a :  ==: 120 ", false, true},
	{"Num1:!=:-120", false, false},
	{"Num1:!=:12 0", false, true},
	{"Num1:>=:100", false, false},
	{"Num1:>=:-200", true, false},
	{"Num1:! =:120", false, true},
	{"Num1:<=:-100", true, false},
	{"Num1:>:100", false, false},
	{"Num1:<:-100", true, false},
	{"Num1:*&:100", false, true},
	{"Num1:in:100", false, true},
	{"Num1:==:abd", false, true},
	{"Num1:==:111abd", false, true},
	{"Num1:in:-120 130", true, false},
	{"Num1:notin:120 130", true, false},
	{"Num1:is:60 -120 130", true, false},
	{"Num1:not:60 33 120 130", true, false},
	//uint
	{"Num_uint1:==:111abd", false, true},
	{"Num_uint1:>=:111abd", true, true},
	//float
	{"Float1:>=:11.2", true, false},
	{"Float1:>=:-11.2", true, false},
	{"Float1:<:100", true, false},
	{"Float1:>=:111abd", false, true},
	//string
	{`Str1:same:"test2" : "test1"`, true, false},
	{`Str1:same:"test1" : "test2"`, true, false},
	{`Str1:differ:"test3" "test2"`, true, false},
	{`Str2:same:"\"inside\"outside" "test2"`, true, false},
	{`Str1:contain:"test" "test2"`, true, false},
	{`Str2:same:"\"inside\"" "test2"`, false, false},
	{`Str2:contain:"\"inside\"" "test2" ""`, true, false},
	{`Str1:same:test1 "test2"`, false, false},
	{`Str1:same:test1 `, false, true},
	//duration
	{"Duration1:==:0m10s", true, false},
	{"Duration1:==:10s", true, false},
	{"Duration1:<:1h", true, false},
	{"Duration1:==:0m10sm", false, true},
	{"Duration1:==:100", false, true},
	{"Duration1:in:1s 1h", true, false},
	{"Duration1:notin:1h 2h", true, false},
	{"Duration1:is:10s 2h 3m", true, false},
	{"Duration1:is:2h 10s 3m", true, false},
	//timestamp
	{"Stamp1:==:2020/03/31T15:00:00", true, false},
	{"Stamp1:>:2010/01/31T15:00:00", true, false},
	{"Stamp1:<:2030/12/31T15:00:00", true, false},
	{"Stamp1:is:3030/04/13T15:00:00 2020/03/31T15:00:00 1200/04/13T15:00:00 ", true, false},
	{"Stamp1:in:2020/03/11T15:00:00 2020/04/13T15:00:00 ", true, false},
	//IP
	{"IP1:within:1.1.1.1/32 2.2.2.2/32", true, false},
	{"IP1:within:1.1.1.1 2.2.2.2/32", false, true},
	{"IP1:within:1.1.1.1/24 2.2.2.2/32", true, false},
	{"IP1:within:1.1.1.0/32 2.2.2.2/32", false, false},
	{"IP1:within:1.1.1.99/24 2.2.2.2/32", true, false},
	{"IP2:within:2001:dead::99/64 2002:beef::/128", true, false},
	{"IP2:notwithin:2002:dead::23/64 2002:beef::/128", false, false},
	{"IP1:within:1.1.1.1/32 2001:dead::1/32", true, false},
}

func TestCmpRule(t *testing.T) {
	var err error
	test_struct.Stamp1, err = time.Parse(TIMEFMTSTR, "2020/03/31T15:00:00")
	if err != nil {
		t.Fatal(err)
	}
	cmp := NewDefaultCMPRule()
	var result bool
	for _, tt := range test_list {
		err = cmp.ParseRule(tt.in)
		if err != nil {
			if !tt.expect_err {
				t.Fatal(err)
			} else {
				t.Logf("input: %v, expected err: %v", tt.in, err)
			}
		} else {
			result, err = cmp.Compare(test_struct)
			t.Logf("input: %v; result: %v, err: %v", tt.in, result, err)
			if err != nil {
				if !tt.expect_err {
					t.Fatal(err)
				} else {
					t.Logf("input: %v, expected err: %v", tt.in, err)
				}
			} else {
				if tt.out_bool != result {
					t.Fatal(err)
				}
			}
		}
	}
}
