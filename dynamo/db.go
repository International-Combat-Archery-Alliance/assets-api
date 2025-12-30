package dynamo

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	gsi1 = "GSI1"
)

// DB is the DynamoDB implementation of the asset repository
type DB struct {
	dynamoClient *dynamodb.Client
	tableName    string
}

// NewDB creates a new DynamoDB repository
func NewDB(client *dynamodb.Client, tableName string) *DB {
	return &DB{
		dynamoClient: client,
		tableName:    tableName,
	}
}

// Helper functions for cursor encoding/decoding

func lastEvalKeyToCursor(lastEvalKey map[string]types.AttributeValue) (string, error) {
	keyMap := make(map[string]string)
	for k, v := range lastEvalKey {
		if sv, ok := v.(*types.AttributeValueMemberS); ok {
			keyMap[k] = sv.Value
		}
	}

	jsonBytes, err := json.Marshal(keyMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cursor: %w", err)
	}

	return base64.StdEncoding.EncodeToString(jsonBytes), nil
}

func cursorToLastEval(cursor string) (map[string]types.AttributeValue, error) {
	jsonBytes, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cursor: %w", err)
	}

	var keyMap map[string]string
	if err := json.Unmarshal(jsonBytes, &keyMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	result := make(map[string]types.AttributeValue)
	for k, v := range keyMap {
		result[k] = &types.AttributeValueMemberS{Value: v}
	}

	return result, nil
}

func getKeyFromItem(lastEvalKey, item map[string]types.AttributeValue) map[string]types.AttributeValue {
	result := make(map[string]types.AttributeValue)
	for key := range lastEvalKey {
		if val, ok := item[key]; ok {
			result[key] = val
		}
	}
	return result
}

func exprMustBuild(builder expression.Builder) expression.Expression {
	expr, err := builder.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build expression: %s", err))
	}
	return expr
}
