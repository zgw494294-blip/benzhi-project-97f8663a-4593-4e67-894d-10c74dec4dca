package main

import "testing"

func TestResolveAddressPriority(t *testing.T) {
	cases := []struct{ explicit, port, want string }{
		{"127.0.0.1:19111", "19222", "127.0.0.1:19111"},
		{"", "19222", "127.0.0.1:19222"},
		{"", "", defaultAddr},
	}
	for _, item := range cases {
		got, err := resolveAddress(item.explicit, item.port)
		if err != nil || got != item.want {
			t.Fatalf("resolveAddress(%q,%q) = %q,%v", item.explicit, item.port, got, err)
		}
	}
	if _, err := resolveAddress("", "8080x"); err == nil {
		t.Fatal("非法 PORT 应被拒绝")
	}
}
