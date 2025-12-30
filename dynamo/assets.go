package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var _ assets.Repository = &DB{}

// assetDynamo is the DynamoDB representation of an asset
type assetDynamo struct {
	PK          string  `dynamodbav:"PK"`
	SK          string  `dynamodbav:"SK"`
	GSI1PK      string  `dynamodbav:"GSI1PK"`
	GSI1SK      string  `dynamodbav:"GSI1SK"`
	ID          string  `dynamodbav:"ID"`
	Folder      string  `dynamodbav:"Folder"`
	Name        string  `dynamodbav:"Name"`
	Description *string `dynamodbav:"Description,omitempty"`
	ContentType string  `dynamodbav:"ContentType"`
	Size        int64   `dynamodbav:"Size"`
	S3Key       string  `dynamodbav:"S3Key"`
	Status      string  `dynamodbav:"Status"`
	CreatedAt   string  `dynamodbav:"CreatedAt"`
	CreatedBy   string  `dynamodbav:"CreatedBy"`
}

// folderDynamo is the DynamoDB representation of a folder index entry
type folderDynamo struct {
	PK     string `dynamodbav:"PK"`
	SK     string `dynamodbav:"SK"`
	Folder string `dynamodbav:"Folder"`
}

const (
	assetEntityName  = "ASSET"
	folderEntityName = "FOLDER"
	foldersIndexPK   = "FOLDERS"
)

func assetPK(id uuid.UUID) string {
	return fmt.Sprintf("%s#%s", assetEntityName, id)
}

func assetSK(id uuid.UUID) string {
	return fmt.Sprintf("%s#%s", assetEntityName, id)
}

func folderGSI1PK(folder string) string {
	return fmt.Sprintf("%s#%s", folderEntityName, folder)
}

func folderGSI1SK(createdAt time.Time, id uuid.UUID) string {
	return fmt.Sprintf("CREATED#%s#%s#%s", createdAt.UTC().Format(time.RFC3339Nano), assetEntityName, id)
}

func folderIndexPK() string {
	return foldersIndexPK
}

func folderIndexSK(folder string) string {
	return fmt.Sprintf("%s#%s", folderEntityName, folder)
}

func newAssetDynamo(asset assets.Asset) assetDynamo {
	return assetDynamo{
		PK:          assetPK(asset.ID),
		SK:          assetSK(asset.ID),
		GSI1PK:      folderGSI1PK(asset.Folder),
		GSI1SK:      folderGSI1SK(asset.CreatedAt, asset.ID),
		ID:          asset.ID.String(),
		Folder:      asset.Folder,
		Name:        asset.Name,
		Description: asset.Description,
		ContentType: asset.ContentType,
		Size:        asset.Size,
		S3Key:       asset.S3Key,
		Status:      string(asset.Status),
		CreatedAt:   asset.CreatedAt.UTC().Format(time.RFC3339Nano),
		CreatedBy:   asset.CreatedBy,
	}
}

func assetFromDynamo(d assetDynamo) assets.Asset {
	createdAt, _ := time.Parse(time.RFC3339Nano, d.CreatedAt)
	return assets.Asset{
		ID:          uuid.MustParse(d.ID),
		Folder:      d.Folder,
		Name:        d.Name,
		Description: d.Description,
		ContentType: d.ContentType,
		Size:        d.Size,
		S3Key:       d.S3Key,
		Status:      assets.Status(d.Status),
		CreatedAt:   createdAt,
		CreatedBy:   d.CreatedBy,
	}
}

// GetAsset retrieves a single asset by ID
func (d *DB) GetAsset(ctx context.Context, id uuid.UUID) (assets.Asset, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: assetPK(id)},
			"SK": &types.AttributeValueMemberS{Value: assetSK(id)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return assets.Asset{}, assets.NewTimeoutError("GetAsset timed out")
		}
		return assets.Asset{}, assets.NewFailedToFetchError(fmt.Sprintf("failed to fetch asset with ID %q", id), err)
	}

	if len(resp.Item) == 0 {
		return assets.Asset{}, assets.NewNotFoundError(fmt.Sprintf("asset with ID %q not found", id), nil)
	}

	var asset assetDynamo
	if err := attributevalue.UnmarshalMap(resp.Item, &asset); err != nil {
		panic(fmt.Sprintf("failed to unmarshal asset from DB: %s", err))
	}

	return assetFromDynamo(asset), nil
}

// GetAssets retrieves assets with optional folder filter and pagination
func (d *DB) GetAssets(ctx context.Context, folder *string, limit int32, cursor *string) (assets.GetAssetsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	var startKey map[string]types.AttributeValue
	var err error
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return assets.GetAssetsResponse{}, assets.NewInvalidCursorError("invalid cursor", err)
		}
	}

	var items []map[string]types.AttributeValue
	var lastEvaluatedKey map[string]types.AttributeValue

	if folder != nil {
		// Query by folder using GSI1
		keyCond := expression.Key("GSI1PK").Equal(expression.Value(folderGSI1PK(*folder)))
		expr := exprMustBuild(expression.NewBuilder().WithKeyCondition(keyCond))

		result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(d.tableName),
			IndexName:                 aws.String(gsi1),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ScanIndexForward:          aws.Bool(false), // Newest first
			Limit:                     aws.Int32(limit + 1),
			ExclusiveStartKey:         startKey,
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return assets.GetAssetsResponse{}, assets.NewTimeoutError("GetAssets timed out")
			}
			return assets.GetAssetsResponse{}, assets.NewFailedToFetchError("failed to fetch assets", err)
		}
		items = result.Items
		lastEvaluatedKey = result.LastEvaluatedKey
	} else {
		// Scan all assets (filter by PK prefix)
		filter := expression.Name("PK").BeginsWith(assetEntityName + "#")
		expr := exprMustBuild(expression.NewBuilder().WithFilter(filter))

		result, err := d.dynamoClient.Scan(ctx, &dynamodb.ScanInput{
			TableName:                 aws.String(d.tableName),
			FilterExpression:          expr.Filter(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Limit:                     aws.Int32(limit + 1),
			ExclusiveStartKey:         startKey,
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return assets.GetAssetsResponse{}, assets.NewTimeoutError("GetAssets timed out")
			}
			return assets.GetAssetsResponse{}, assets.NewFailedToFetchError("failed to fetch assets", err)
		}
		items = result.Items
		lastEvaluatedKey = result.LastEvaluatedKey
	}

	var dynamoItems []assetDynamo
	if err := attributevalue.UnmarshalListOfMaps(items, &dynamoItems); err != nil {
		panic(fmt.Sprintf("failed to unmarshal assets: %s", err))
	}

	hasNextPage := len(dynamoItems) > int(limit)

	var newCursor *string
	if hasNextPage && len(lastEvaluatedKey) > 0 {
		lastItemGivenToUser := items[len(items)-2]
		lastItemKey := getKeyFromItem(lastEvaluatedKey, lastItemGivenToUser)
		c, err := lastEvalKeyToCursor(lastItemKey)
		if err != nil {
			panic(fmt.Sprintf("failed to make cursor from lastEvalKey: %s", err))
		}
		newCursor = &c
	}

	data := make([]assets.Asset, 0, min(int(limit), len(dynamoItems)))
	for i := 0; i < min(int(limit), len(dynamoItems)); i++ {
		data = append(data, assetFromDynamo(dynamoItems[i]))
	}

	return assets.GetAssetsResponse{
		Data:        data,
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}

// GetFolders retrieves all distinct folder names
func (d *DB) GetFolders(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("PK").Equal(expression.Value(folderIndexPK()))
	expr := exprMustBuild(expression.NewBuilder().WithKeyCondition(keyCond))

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, assets.NewTimeoutError("GetFolders timed out")
		}
		return nil, assets.NewFailedToFetchError("failed to fetch folders", err)
	}

	var folders []folderDynamo
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &folders); err != nil {
		panic(fmt.Sprintf("failed to unmarshal folders: %s", err))
	}

	folderNames := make([]string, len(folders))
	for i, f := range folders {
		folderNames[i] = f.Folder
	}

	return folderNames, nil
}

// CreateAsset creates a new asset record
func (d *DB) CreateAsset(ctx context.Context, asset assets.Asset) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoItem := newAssetDynamo(asset)

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return assets.NewFailedToWriteError("failed to marshal asset", err)
	}

	// Condition to ensure asset doesn't already exist
	cond := expression.AttributeNotExists(expression.Name("PK"))
	expr := exprMustBuild(expression.NewBuilder().WithCondition(cond))

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                 aws.String(d.tableName),
		Item:                      item,
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		var condCheckFailedErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailedErr) {
			return assets.NewAlreadyExistsError(fmt.Sprintf("asset with ID %q already exists", asset.ID), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("CreateAsset timed out")
		}
		return assets.NewFailedToWriteError("failed to create asset", err)
	}

	return nil
}

// UpdateAsset updates an existing asset
func (d *DB) UpdateAsset(ctx context.Context, asset assets.Asset) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoItem := newAssetDynamo(asset)

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return assets.NewFailedToWriteError("failed to marshal asset", err)
	}

	// Condition to ensure asset exists
	cond := expression.AttributeExists(expression.Name("PK"))
	expr := exprMustBuild(expression.NewBuilder().WithCondition(cond))

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                 aws.String(d.tableName),
		Item:                      item,
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		var condCheckFailedErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailedErr) {
			return assets.NewNotFoundError(fmt.Sprintf("asset with ID %q not found", asset.ID), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("UpdateAsset timed out")
		}
		return assets.NewFailedToWriteError("failed to update asset", err)
	}

	return nil
}

// DeleteAsset deletes an asset by ID
func (d *DB) DeleteAsset(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Condition to ensure asset exists
	cond := expression.AttributeExists(expression.Name("PK"))
	expr := exprMustBuild(expression.NewBuilder().WithCondition(cond))

	_, err := d.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: assetPK(id)},
			"SK": &types.AttributeValueMemberS{Value: assetSK(id)},
		},
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		var condCheckFailedErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailedErr) {
			return assets.NewNotFoundError(fmt.Sprintf("asset with ID %q not found", id), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("DeleteAsset timed out")
		}
		return assets.NewFailedToDeleteError("failed to delete asset", err)
	}

	return nil
}

// AddFolder adds a folder to the folder index
func (d *DB) AddFolder(ctx context.Context, folder string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	item := folderDynamo{
		PK:     folderIndexPK(),
		SK:     folderIndexSK(folder),
		Folder: folder,
	}

	dynamoItem, err := attributevalue.MarshalMap(item)
	if err != nil {
		return assets.NewFailedToWriteError("failed to marshal folder", err)
	}

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      dynamoItem,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("AddFolder timed out")
		}
		return assets.NewFailedToWriteError("failed to add folder", err)
	}

	return nil
}

// RemoveFolder removes a folder from the index
func (d *DB) RemoveFolder(ctx context.Context, folder string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := d.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: folderIndexPK()},
			"SK": &types.AttributeValueMemberS{Value: folderIndexSK(folder)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("RemoveFolder timed out")
		}
		return assets.NewFailedToDeleteError("failed to remove folder", err)
	}

	return nil
}

// CountAssetsInFolder returns the number of assets in a folder
func (d *DB) CountAssetsInFolder(ctx context.Context, folder string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("GSI1PK").Equal(expression.Value(folderGSI1PK(folder)))
	expr := exprMustBuild(expression.NewBuilder().WithKeyCondition(keyCond))

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		IndexName:                 aws.String(gsi1),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Select:                    types.SelectCount,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, assets.NewTimeoutError("CountAssetsInFolder timed out")
		}
		return 0, assets.NewFailedToFetchError("failed to count assets in folder", err)
	}

	return int(result.Count), nil
}
