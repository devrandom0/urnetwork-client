package main

import "testing"

func TestSplitCSV(t *testing.T) {
    cases := []struct{
        in string
        want []string
    }{
        {"", nil},
        {"  ", nil},
        {"a", []string{"a"}},
        {"a,b", []string{"a","b"}},
        {" a , b , ", []string{"a","b"}},
        {",,a,,b,,", []string{"a","b"}},
    }
    for _, c := range cases {
        got := splitCSV(c.in)
        if len(got) != len(c.want) {
            t.Fatalf("splitCSV(%q) len=%d want=%d", c.in, len(got), len(c.want))
        }
        for i := range got {
            if got[i] != c.want[i] {
                t.Fatalf("splitCSV(%q)[%d]=%q want %q", c.in, i, got[i], c.want[i])
            }
        }
    }
}
