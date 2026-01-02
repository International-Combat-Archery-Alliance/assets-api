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

var _ assets.MetadataRepository = &DB{}

const (
	assetTypeFile   = "FILE"
	assetTypeFolder = "FOLDER"
)

// assetDynamo is the unified DynamoDB representation of an asset (file or folder)
type assetDynamo struct {
	PK          string  `dynamodbav:"PK"`
	SK          string  `dynamodbav:"SK"`
	GSI1PK      string  `dynamodbav:"GSI1PK"`
	GSI1SK      string  `dynamodbav:"GSI1SK"`
	ID          string  `dynamodbav:"ID"`
	Type        string  `dynamodbav:"Type"`
	Path        string  `dynamodbav:"Path"`
	Name        string  `dynamodbav:"Name"`
	Description *string `dynamodbav:"Description,omitempty"`
	CreatedAt   string  `dynamodbav:"CreatedAt"`
	CreatedBy   string  `dynamodbav:"CreatedBy"`
	Version     int     `dynamodbav:"Version"`

	// File-specific fields
	ContentType *string `dynamodbav:"ContentType,omitempty"`
	Size        *int64  `dynamodbav:"Size,omitempty"`
	ObjectKey   *string `dynamodbav:"ObjectKey,omitempty"`
	Status      *string `dynamodbav:"Status,omitempty"`

	// Folder-specific fields
	ContentCount *int `dynamodbav:"ContentCount,omitempty"`
}

func assetPK(id uuid.UUID) string {
	return fmt.Sprintf("ASSET#%s", id)
}

func assetSK(id uuid.UUID) string {
	return fmt.Sprintf("ASSET#%s", id)
}

func pathGSI1PK(path string) string {
	return fmt.Sprintf("PATH#%s", path)
}

func pathGSI1SK(createdAt time.Time, assetType string, id uuid.UUID) string {
	return fmt.Sprintf("CREATED#%s#%s#%s", createdAt.UTC().Format(time.RFC3339Nano), assetType, id)
}

func newFileDynamo(file *assets.File) assetDynamo {
	return assetDynamo{
		PK:          assetPK(file.ID),
		SK:          assetSK(file.ID),
		GSI1PK:      pathGSI1PK(file.Path),
		GSI1SK:      pathGSI1SK(file.CreatedAt, assetTypeFile, file.ID),
		ID:          file.ID.String(),
		Type:        assetTypeFile,
		Path:        file.Path,
		Name:        file.Name,
		Description: file.Description,
		CreatedAt:   file.CreatedAt.UTC().Format(time.RFC3339Nano),
		CreatedBy:   file.CreatedBy,
		Version:     file.Version,
		ContentType: &file.ContentType,
		Size:        &file.Size,
		ObjectKey:   &file.ObjectKey,
		Status:      ptrString(string(file.Status)),
	}
}

func newFolderDynamo(folder *assets.Folder) assetDynamo {
	return assetDynamo{
		PK:           assetPK(folder.ID),
		SK:           assetSK(folder.ID),
		GSI1PK:       pathGSI1PK(folder.Path),
		GSI1SK:       pathGSI1SK(folder.CreatedAt, assetTypeFolder, folder.ID),
		ID:           folder.ID.String(),
		Type:         assetTypeFolder,
		Path:         folder.Path,
		Name:         folder.Name,
		Description:  folder.Description,
		CreatedAt:    folder.CreatedAt.UTC().Format(time.RFC3339Nano),
		CreatedBy:    folder.CreatedBy,
		Version:      folder.Version,
		ContentCount: &folder.ContentCount,
	}
}

func assetFromDynamo(d assetDynamo) assets.Asset {
	createdAt, _ := time.Parse(time.RFC3339Nano, d.CreatedAt)

	if d.Type == assetTypeFile {
		file := &assets.File{
			ID:          uuid.MustParse(d.ID),
			Path:        d.Path,
			Name:        d.Name,
			Description: d.Description,
			CreatedAt:   createdAt,
			CreatedBy:   d.CreatedBy,
			Version:     d.Version,
		}
		if d.ContentType != nil {
			file.ContentType = *d.ContentType
		}
		if d.Size != nil {
			file.Size = *d.Size
		}
		if d.ObjectKey != nil {
			file.ObjectKey = *d.ObjectKey
		}
		if d.Status != nil {
			file.Status = assets.Status(*d.Status)
		}
		return file
	}

	folder := &assets.Folder{
		ID:          uuid.MustParse(d.ID),
		Path:        d.Path,
		Name:        d.Name,
		Description: d.Description,
		CreatedAt:   createdAt,
		CreatedBy:   d.CreatedBy,
		Version:     d.Version,
	}
	if d.ContentCount != nil {
		folder.ContentCount = *d.ContentCount
	}
	return folder
}

func ptrString(s string) *string {
	return &s
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
			return nil, assets.NewTimeoutError("GetAsset timed out")
		}
		return nil, assets.NewFailedToFetchError(fmt.Sprintf("failed to fetch asset with ID %q", id), err)
	}

	if len(resp.Item) == 0 {
		return nil, assets.NewNotFoundError(fmt.Sprintf("asset with ID %q not found", id), nil)
	}

	var asset assetDynamo
	if err := attributevalue.UnmarshalMap(resp.Item, &asset); err != nil {
		panic(fmt.Sprintf("failed to unmarshal asset from DB: %s", err))
	}

	return assetFromDynamo(asset), nil
}

// GetAssets retrieves assets at a specific path with pagination
func (d *DB) GetAssets(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error) {
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

	// Query by path using GSI1
	keyCond := expression.Key("GSI1PK").Equal(expression.Value(pathGSI1PK(path)))
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

	var dynamoItems []assetDynamo
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems); err != nil {
		panic(fmt.Sprintf("failed to unmarshal assets: %s", err))
	}

	hasNextPage := len(dynamoItems) > int(limit)

	var newCursor *string
	if hasNextPage && len(result.LastEvaluatedKey) > 0 {
		lastItemGivenToUser := result.Items[len(result.Items)-2]
		lastItemKey := getKeyFromItem(result.LastEvaluatedKey, lastItemGivenToUser)
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

// CreateAsset creates a new asset record (file or folder)
func (d *DB) CreateAsset(ctx context.Context, asset assets.Asset) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dynamoItem assetDynamo
	var parentPath string

	switch a := asset.(type) {
	case *assets.File:
		a.Version = 1 // Initialize version for new assets
		dynamoItem = newFileDynamo(a)
		parentPath = a.Path
	case *assets.Folder:
		a.Version = 1 // Initialize version for new assets
		dynamoItem = newFolderDynamo(a)
		parentPath = a.Path
	default:
		return assets.NewFailedToWriteError("unknown asset type", nil)
	}

	// Determine parent folder ID
	var parentID uuid.UUID
	if parentPath == assets.RootPath {
		// Creating at root - use root folder as parent
		parentID = assets.RootFolderID
	} else {
		// Validate parent folder exists
		exists, id, err := d.folderExistsAtPath(ctx, parentPath)
		if err != nil {
			return err
		}
		if !exists {
			return assets.NewParentFolderNotFoundError(fmt.Sprintf("parent folder at path %q not found", parentPath))
		}
		parentID = id
	}

	// Use a transaction to create the asset and increment the parent's ContentCount
	return d.createAssetWithContentCountUpdate(ctx, dynamoItem, parentID, 1)
}

// UpdateAsset updates an existing asset with optimistic locking.
// The caller must increment the version before calling this method.
// The repository will check that the current DB version is one less than the passed version.
func (d *DB) UpdateAsset(ctx context.Context, asset assets.Asset) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	var dynamoItem assetDynamo
	var newVersion int
	var assetID string

	switch a := asset.(type) {
	case *assets.File:
		newVersion = a.Version
		dynamoItem = newFileDynamo(a)
		assetID = a.ID.String()
	case *assets.Folder:
		newVersion = a.Version
		dynamoItem = newFolderDynamo(a)
		assetID = a.ID.String()
	default:
		return assets.NewFailedToWriteError("unknown asset type", nil)
	}

	// The expected version in DB is one less than what we're saving
	expectedDBVersion := newVersion - 1

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return assets.NewFailedToWriteError("failed to marshal asset", err)
	}

	// Condition: asset must exist AND current DB version must be one less than new version (optimistic locking)
	cond := expression.And(
		expression.AttributeExists(expression.Name("PK")),
		expression.Name("Version").Equal(expression.Value(expectedDBVersion)),
	)
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
			// Check if the asset exists to distinguish between not found and version conflict
			existingAsset, getErr := d.GetAsset(ctx, uuid.MustParse(assetID))
			if getErr != nil {
				if assets.IsNotFoundError(getErr) {
					return assets.NewNotFoundError(fmt.Sprintf("asset with ID %q not found", assetID), err)
				}
				return getErr
			}
			// Asset exists but version didn't match - version conflict
			var existingVersion int
			switch a := existingAsset.(type) {
			case *assets.File:
				existingVersion = a.Version
			case *assets.Folder:
				existingVersion = a.Version
			}
			return assets.NewVersionConflictError(
				fmt.Sprintf("version conflict: expected version %d but found version %d", expectedDBVersion, existingVersion),
			)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("UpdateAsset timed out")
		}
		return assets.NewFailedToWriteError("failed to update asset", err)
	}

	return nil
}

// DeleteAsset deletes an asset by ID
func (d *DB) DeleteAsset(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Prevent deletion of the root folder
	if id == assets.RootFolderID {
		return assets.NewNotAllowedToDeleteRootError()
	}

	// First get the asset to determine its type and path
	asset, err := d.GetAsset(ctx, id)
	if err != nil {
		return err
	}

	var parentPath string

	switch a := asset.(type) {
	case *assets.File:
		parentPath = a.Path
	case *assets.Folder:
		parentPath = a.Path
		if !a.IsEmpty() {
			return assets.NewFolderNotEmptyError(fmt.Sprintf("folder %q is not empty (contains %d items)", a.Name, a.ContentCount))
		}
	}

	// Determine parent folder ID
	var parentID uuid.UUID
	if parentPath == assets.RootPath {
		// Deleting from root - use root folder as parent
		parentID = assets.RootFolderID
	} else {
		// Find parent folder
		exists, id, err := d.folderExistsAtPath(ctx, parentPath)
		if err != nil {
			return err
		}
		if !exists {
			// Parent doesn't exist (orphaned asset), just delete without ContentCount update
			return d.deleteAssetSimple(ctx, id)
		}
		parentID = id
	}

	// Delete asset and decrement parent's ContentCount
	return d.deleteAssetWithContentCountUpdate(ctx, id, parentID, -1)
}

// deleteAssetSimple deletes an asset without updating any parent's ContentCount
func (d *DB) deleteAssetSimple(ctx context.Context, id uuid.UUID) error {
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

// folderExistsAtPath checks if a folder exists at the given path and returns its ID
// The path is the full path of the folder, e.g., "/images/carousel"
// We need to find a folder whose Path + "/" + Name equals the given fullPath
func (d *DB) folderExistsAtPath(ctx context.Context, fullPath string) (bool, uuid.UUID, error) {
	// Parse the path to get parent path and folder name
	parentPath, folderName := splitPath(fullPath)

	// Query GSI1 for folders at parentPath with the matching name
	keyCond := expression.Key("GSI1PK").Equal(expression.Value(pathGSI1PK(parentPath)))
	filter := expression.And(
		expression.Name("Type").Equal(expression.Value(assetTypeFolder)),
		expression.Name("Name").Equal(expression.Value(folderName)),
	)
	expr := exprMustBuild(expression.NewBuilder().WithKeyCondition(keyCond).WithFilter(filter))

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		IndexName:                 aws.String(gsi1),
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(1),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, uuid.Nil, assets.NewTimeoutError("folderExistsAtPath timed out")
		}
		return false, uuid.Nil, assets.NewFailedToFetchError("failed to check folder existence", err)
	}

	if len(result.Items) == 0 {
		return false, uuid.Nil, nil
	}

	var folder assetDynamo
	if err := attributevalue.UnmarshalMap(result.Items[0], &folder); err != nil {
		panic(fmt.Sprintf("failed to unmarshal folder: %s", err))
	}

	return true, uuid.MustParse(folder.ID), nil
}

// createAssetWithContentCountUpdate creates an asset and increments the parent folder's ContentCount
func (d *DB) createAssetWithContentCountUpdate(ctx context.Context, asset assetDynamo, parentID uuid.UUID, delta int) error {
	item, err := attributevalue.MarshalMap(asset)
	if err != nil {
		return assets.NewFailedToWriteError("failed to marshal asset", err)
	}

	// Build the update expression for ContentCount
	updateExpr := expression.Add(expression.Name("ContentCount"), expression.Value(delta))
	parentCond := expression.AttributeExists(expression.Name("PK"))
	parentExpr := exprMustBuild(expression.NewBuilder().WithUpdate(updateExpr).WithCondition(parentCond))

	// Build the condition for the new asset
	assetCond := expression.AttributeNotExists(expression.Name("PK"))
	assetExpr := exprMustBuild(expression.NewBuilder().WithCondition(assetCond))

	_, err = d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:                 aws.String(d.tableName),
					Item:                      item,
					ConditionExpression:       assetExpr.Condition(),
					ExpressionAttributeNames:  assetExpr.Names(),
					ExpressionAttributeValues: assetExpr.Values(),
				},
			},
			{
				Update: &types.Update{
					TableName: aws.String(d.tableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: assetPK(parentID)},
						"SK": &types.AttributeValueMemberS{Value: assetSK(parentID)},
					},
					UpdateExpression:          parentExpr.Update(),
					ConditionExpression:       parentExpr.Condition(),
					ExpressionAttributeNames:  parentExpr.Names(),
					ExpressionAttributeValues: parentExpr.Values(),
				},
			},
		},
	})
	if err != nil {
		var txErr *types.TransactionCanceledException
		if errors.As(err, &txErr) {
			// Check which operation failed
			for i, reason := range txErr.CancellationReasons {
				if reason.Code != nil && *reason.Code == "ConditionalCheckFailed" {
					if i == 0 {
						return assets.NewAlreadyExistsError("asset already exists", err)
					}
					return assets.NewParentFolderNotFoundError("parent folder not found")
				}
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("createAssetWithContentCountUpdate timed out")
		}
		return assets.NewFailedToWriteError("failed to create asset", err)
	}

	return nil
}

// deleteAssetWithContentCountUpdate deletes an asset and decrements the parent folder's ContentCount
func (d *DB) deleteAssetWithContentCountUpdate(ctx context.Context, assetID, parentID uuid.UUID, delta int) error {
	// Build the delete condition
	deleteCond := expression.AttributeExists(expression.Name("PK"))
	deleteExpr := exprMustBuild(expression.NewBuilder().WithCondition(deleteCond))

	// Build the update expression for ContentCount
	updateExpr := expression.Add(expression.Name("ContentCount"), expression.Value(delta))
	parentCond := expression.AttributeExists(expression.Name("PK"))
	parentExpr := exprMustBuild(expression.NewBuilder().WithUpdate(updateExpr).WithCondition(parentCond))

	_, err := d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Delete: &types.Delete{
					TableName: aws.String(d.tableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: assetPK(assetID)},
						"SK": &types.AttributeValueMemberS{Value: assetSK(assetID)},
					},
					ConditionExpression:       deleteExpr.Condition(),
					ExpressionAttributeNames:  deleteExpr.Names(),
					ExpressionAttributeValues: deleteExpr.Values(),
				},
			},
			{
				Update: &types.Update{
					TableName: aws.String(d.tableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: assetPK(parentID)},
						"SK": &types.AttributeValueMemberS{Value: assetSK(parentID)},
					},
					UpdateExpression:          parentExpr.Update(),
					ConditionExpression:       parentExpr.Condition(),
					ExpressionAttributeNames:  parentExpr.Names(),
					ExpressionAttributeValues: parentExpr.Values(),
				},
			},
		},
	})
	if err != nil {
		var txErr *types.TransactionCanceledException
		if errors.As(err, &txErr) {
			for i, reason := range txErr.CancellationReasons {
				if reason.Code != nil && *reason.Code == "ConditionalCheckFailed" {
					if i == 0 {
						return assets.NewNotFoundError(fmt.Sprintf("asset with ID %q not found", assetID), err)
					}
					// Parent doesn't exist, but we still want to delete the asset
					// Fall through to simple delete
				}
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("deleteAssetWithContentCountUpdate timed out")
		}
		return assets.NewFailedToDeleteError("failed to delete asset", err)
	}

	return nil
}

// splitPath splits a full path into parent path and name
// e.g., "/images/carousel" -> ("/images", "carousel")
// e.g., "/images" -> ("/", "images")
func splitPath(fullPath string) (parentPath, name string) {
	if fullPath == "" || fullPath == "/" {
		return "/", ""
	}

	// Remove trailing slash if present
	if fullPath[len(fullPath)-1] == '/' {
		fullPath = fullPath[:len(fullPath)-1]
	}

	// Find the last slash
	lastSlash := -1
	for i := len(fullPath) - 1; i >= 0; i-- {
		if fullPath[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash == -1 {
		return "/", fullPath
	}

	if lastSlash == 0 {
		return "/", fullPath[1:]
	}

	return fullPath[:lastSlash], fullPath[lastSlash+1:]
}

// EnsureRootFolderExists creates the root folder if it doesn't exist.
// The root folder has a well-known ID (assets.RootFolderID), Path="" (no parent), and Name="/".
func (d *DB) EnsureRootFolderExists(ctx context.Context, createdBy string) (*assets.Folder, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Try to get the root folder first
	existing, err := d.GetAsset(ctx, assets.RootFolderID)
	if err == nil {
		// Root folder exists
		if folder, ok := existing.(*assets.Folder); ok {
			return folder, nil
		}
		// This shouldn't happen - root folder ID is used for something else
		return nil, assets.NewFailedToWriteError("root folder ID is not a folder", nil)
	}

	if !assets.IsNotFoundError(err) {
		return nil, err
	}

	// Root folder doesn't exist, create it
	now := time.Now().UTC()
	rootFolder := &assets.Folder{
		ID:           assets.RootFolderID,
		Path:         "", // No parent
		Name:         "/",
		ContentCount: 0,
		Description:  nil,
		CreatedAt:    now,
		CreatedBy:    createdBy,
		Version:      1,
	}

	dynamoItem := newFolderDynamo(rootFolder)
	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return nil, assets.NewFailedToWriteError("failed to marshal root folder", err)
	}

	// Use condition to avoid overwriting if it was created concurrently
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
			// Created concurrently, fetch and return it
			existing, err := d.GetAsset(ctx, assets.RootFolderID)
			if err != nil {
				return nil, err
			}
			if folder, ok := existing.(*assets.Folder); ok {
				return folder, nil
			}
			return nil, assets.NewFailedToWriteError("root folder ID is not a folder", nil)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, assets.NewTimeoutError("EnsureRootFolderExists timed out")
		}
		return nil, assets.NewFailedToWriteError("failed to create root folder", err)
	}

	return rootFolder, nil
}
