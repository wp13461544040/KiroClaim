package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDispatchHealthCheckSettingDefaultsToEnabledWhenStoredValueMissing(t *testing.T) {
	settings := AppSettings{DispatchHealthCheckEnabled: true}

	mergeStoredRuntimeSettings(&settings, storedRuntimeSettings{})

	if !settings.DispatchHealthCheckEnabled {
		t.Fatal("missing stored value should preserve the enabled default")
	}
}

func TestDispatchHealthCheckSettingAcceptsAndSerializesDisabledValue(t *testing.T) {
	settings := AppSettings{DispatchHealthCheckEnabled: true}
	stored := storedRuntimeSettings{DispatchHealthCheckEnabled: boolPtr(false)}

	mergeStoredRuntimeSettings(&settings, stored)

	if settings.DispatchHealthCheckEnabled {
		t.Fatal("explicit stored false should disable dispatch health checks")
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal stored settings: %v", err)
	}
	if !strings.Contains(string(payload), `"dispatchHealthCheckEnabled":false`) {
		t.Fatalf("serialized settings missing disabled value: %s", payload)
	}
}
