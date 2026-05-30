package main

import "testing"

func TestSelectPostgresSchema(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{name: "v prefix", version: "v11.0.0"},
		{name: "patch prerelease tolerant", version: "11.1.0-dev"},
		{name: "unsupported old version", version: "10.12.0", wantErr: true},
		{name: "invalid version", version: "not-a-version", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := selectPostgresSchema(tt.version)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !tt.wantErr {
				if _, ok := schema.(postgresSchemaV11); !ok {
					t.Fatalf("expected postgresSchemaV11, got %T", schema)
				}
			}
		})
	}
}

func TestNewBackfillStoreRejectsNonPostgres(t *testing.T) {
	_, err := newBackfillStore(nil, "mysql", "11.0.0")
	if err == nil {
		t.Fatal("expected unsupported driver error")
	}
}
