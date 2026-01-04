package dynamo

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestLastEvalKeyToCursor(t *testing.T) {
	lastEvalKey := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "PATH#/"},
		"SK": &types.AttributeValueMemberS{Value: "NAME#test"},
	}

	cursor, err := lastEvalKeyToCursor(lastEvalKey)
	if err != nil {
		t.Fatalf("lastEvalKeyToCursor() error = %v", err)
	}

	if cursor == "" {
		t.Error("cursor should not be empty")
	}

	jsonBytes, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatalf("Failed to decode cursor: %v", err)
	}

	var keyMap map[string]string
	if err := json.Unmarshal(jsonBytes, &keyMap); err != nil {
		t.Fatalf("Failed to unmarshal cursor: %v", err)
	}

	if keyMap["PK"] != "PATH#/" {
		t.Errorf("PK = %q, want %q", keyMap["PK"], "PATH#/")
	}
	if keyMap["SK"] != "NAME#test" {
		t.Errorf("SK = %q, want %q", keyMap["SK"], "NAME#test")
	}
}

func TestCursorToLastEval(t *testing.T) {
	keyMap := map[string]string{
		"PK": "PATH#/",
		"SK": "NAME#test",
	}

	jsonBytes, err := json.Marshal(keyMap)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	cursor := base64.StdEncoding.EncodeToString(jsonBytes)

	lastEvalKey, err := cursorToLastEval(cursor)
	if err != nil {
		t.Fatalf("cursorToLastEval() error = %v", err)
	}

	if len(lastEvalKey) != 2 {
		t.Errorf("lastEvalKey length = %d, want 2", len(lastEvalKey))
	}

	pkVal, ok := lastEvalKey["PK"].(*types.AttributeValueMemberS)
	if !ok {
		t.Fatal("PK is not a string attribute")
	}
	if pkVal.Value != "PATH#/" {
		t.Errorf("PK = %q, want %q", pkVal.Value, "PATH#/")
	}

	skVal, ok := lastEvalKey["SK"].(*types.AttributeValueMemberS)
	if !ok {
		t.Fatal("SK is not a string attribute")
	}
	if skVal.Value != "NAME#test" {
		t.Errorf("SK = %q, want %q", skVal.Value, "NAME#test")
	}
}

func TestCursorToLastEval_InvalidBase64(t *testing.T) {
	_, err := cursorToLastEval("invalid-base64!")
	if err == nil {
		t.Error("cursorToLastEval() expected error for invalid base64, got nil")
	}
}

func TestCursorToLastEval_InvalidJSON(t *testing.T) {
	invalidJSON := base64.StdEncoding.EncodeToString([]byte("not json"))
	_, err := cursorToLastEval(invalidJSON)
	if err == nil {
		t.Error("cursorToLastEval() expected error for invalid JSON, got nil")
	}
}

func TestGetKeyFromItem(t *testing.T) {
	lastEvalKey := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "key-pk"},
		"SK": &types.AttributeValueMemberS{Value: "key-sk"},
	}

	item := map[string]types.AttributeValue{
		"PK":   &types.AttributeValueMemberS{Value: "item-pk"},
		"SK":   &types.AttributeValueMemberS{Value: "item-sk"},
		"Type": &types.AttributeValueMemberS{Value: "FILE"},
		"Name": &types.AttributeValueMemberS{Value: "test.txt"},
	}

	result := getKeyFromItem(lastEvalKey, item)

	if len(result) != 2 {
		t.Errorf("result length = %d, want 2", len(result))
	}

	pkVal, ok := result["PK"].(*types.AttributeValueMemberS)
	if !ok {
		t.Fatal("PK is not a string attribute")
	}
	if pkVal.Value != "item-pk" {
		t.Errorf("PK = %q, want %q", pkVal.Value, "item-pk")
	}

	skVal, ok := result["SK"].(*types.AttributeValueMemberS)
	if !ok {
		t.Fatal("SK is not a string attribute")
	}
	if skVal.Value != "item-sk" {
		t.Errorf("SK = %q, want %q", skVal.Value, "item-sk")
	}

	if _, exists := result["Type"]; exists {
		t.Error("result should only contain keys from lastEvalKey")
	}
	if _, exists := result["Name"]; exists {
		t.Error("result should only contain keys from lastEvalKey")
	}
}

func TestPathPK(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"root", "/", "PATH#/"},
		{"simple", "/foo", "PATH#/foo"},
		{"nested", "/foo/bar", "PATH#/foo/bar"},
		{"empty", "", "PATH#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathPK(tt.path)
			if got != tt.want {
				t.Errorf("pathPK(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestNameSK(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"simple", "test.txt", "NAME#test.txt"},
		{"with spaces", "my file.txt", "NAME#my file.txt"},
		{"special chars", "file-1_2.txt", "NAME#file-1_2.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nameSK(tt.path)
			if got != tt.want {
				t.Errorf("nameSK(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRoundTripCursorConversion(t *testing.T) {
	originalKey := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "PATH#/test"},
		"SK": &types.AttributeValueMemberS{Value: "NAME#file.txt"},
	}

	cursor, err := lastEvalKeyToCursor(originalKey)
	if err != nil {
		t.Fatalf("lastEvalKeyToCursor() error = %v", err)
	}

	restoredKey, err := cursorToLastEval(cursor)
	if err != nil {
		t.Fatalf("cursorToLastEval() error = %v", err)
	}

	if len(restoredKey) != len(originalKey) {
		t.Errorf("restored key length = %d, want %d", len(restoredKey), len(originalKey))
	}

	for k, v := range originalKey {
		restoredVal, exists := restoredKey[k]
		if !exists {
			t.Errorf("restored key missing attribute %q", k)
			continue
		}

		origStr, ok1 := v.(*types.AttributeValueMemberS)
		restoredStr, ok2 := restoredVal.(*types.AttributeValueMemberS)

		if !ok1 || !ok2 {
			t.Errorf("type mismatch for attribute %q", k)
			continue
		}

		if origStr.Value != restoredStr.Value {
			t.Errorf("restored value for %q = %q, want %q", k, restoredStr.Value, origStr.Value)
		}
	}
}
