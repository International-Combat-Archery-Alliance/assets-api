package dynamo

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/International-Combat-Archery-Alliance/assets-api/ptr"
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
	TTL         *int64  `dynamodbav:"TTL,omitempty"`

	// Folder-specific fields
	ContentCount *int `dynamodbav:"ContentCount,omitempty"`
}

func pathPK(path string) string {
	return fmt.Sprintf("PATH#%s", path)
}

func nameSK(name string) string {
	return fmt.Sprintf("NAME#%s", name)
}

func newFileDynamo(file *assets.File) assetDynamo {
	var ttl *int64

	if file.ExpiresAt != nil {
		ttl = ptr.Int64(file.ExpiresAt.Unix())
	}

	return assetDynamo{
		PK:          pathPK(file.Path),
		SK:          nameSK(file.Name),
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
		Status:      ptr.String(string(file.Status)),
		TTL:         ttl,
	}
}

func newFolderDynamo(folder *assets.Folder) assetDynamo {
	return assetDynamo{
		PK:           pathPK(folder.Path),
		SK:           nameSK(folder.Name),
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
		if d.TTL != nil {
			file.ExpiresAt = ptr.Time(time.Unix(*d.TTL, 0))
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

// GetAsset retrieves a single asset by full path
func (d *DB) GetAsset(ctx context.Context, fullPath string) (assets.Asset, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	parentPath, name := splitFullPath(fullPath)

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pathPK(parentPath)},
			"SK": &types.AttributeValueMemberS{Value: nameSK(name)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, assets.NewTimeoutError("GetAsset timed out")
		}
		return nil, assets.NewFailedToFetchError(fmt.Sprintf("failed to fetch asset at path %q", fullPath), err)
	}

	if len(resp.Item) == 0 {
		return nil, assets.NewNotFoundError(fmt.Sprintf("asset at path %q not found", fullPath), nil)
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

	// Query by path using main table (PK = PATH#{path})
	keyCond := expression.Key("PK").Equal(expression.Value(pathPK(path)))
	expr := exprMustBuild(expression.NewBuilder().WithKeyCondition(keyCond))

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          aws.Bool(true), // Alphabetical order by name
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

func (d *DB) CreateAsset(ctx context.Context, asset assets.Asset) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var dynamoItem assetDynamo
	var parentPath string

	switch a := asset.(type) {
	case *assets.File:
		dynamoItem = newFileDynamo(a)
		parentPath = a.Path
	case *assets.Folder:
		dynamoItem = newFolderDynamo(a)
		parentPath = a.Path
	default:
		return assets.NewFailedToWriteError("unknown asset type", nil)
	}

	// Use a transaction to create the asset and increment the parent's ContentCount
	// The transaction will verify the parent folder exists
	return d.createAssetWithContentCountUpdate(ctx, dynamoItem, parentPath, 1)
}

// UpdateAsset updates an existing asset with optimistic locking.
// The caller must increment the version before calling this method.
// The repository will check that the current DB version is one less than the passed version.
// NOTE: This does NOT support changing path or name (moves/renames). Those operations are not allowed.
func (d *DB) UpdateAsset(ctx context.Context, asset assets.Asset) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	var dynamoItem assetDynamo
	var newVersion int
	var assetPath, assetName string

	switch a := asset.(type) {
	case *assets.File:
		newVersion = a.Version
		dynamoItem = newFileDynamo(a)
		assetPath = a.Path
		assetName = a.Name
	case *assets.Folder:
		newVersion = a.Version
		dynamoItem = newFolderDynamo(a)
		assetPath = a.Path
		assetName = a.Name
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
	// Also verify path and name haven't changed (disallow moves/renames)
	cond := expression.And(
		expression.AttributeExists(expression.Name("PK")),
		expression.Name("Version").Equal(expression.Value(expectedDBVersion)),
		expression.Name("Path").Equal(expression.Value(assetPath)),
		expression.Name("Name").Equal(expression.Value(assetName)),
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
			// Construct full path for error message using path.Join
			fullPath := path.Join(assetPath, assetName)

			// Check if the asset exists to distinguish between not found and version conflict
			existingAsset, getErr := d.GetAsset(ctx, fullPath)
			if getErr != nil {
				if assets.IsNotFoundError(getErr) {
					return assets.NewNotFoundError(fmt.Sprintf("asset at path %q not found", fullPath), err)
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

// DeleteAsset deletes an asset by full path
func (d *DB) DeleteAsset(ctx context.Context, fullPath string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Prevent deletion of the root folder
	if fullPath == "/" {
		return assets.NewNotAllowedToDeleteRootError()
	}

	// First get the asset to determine its type
	asset, err := d.GetAsset(ctx, fullPath)
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

	// Delete asset and decrement parent's ContentCount using transaction
	return d.deleteAssetWithContentCountUpdate(ctx, fullPath, parentPath, -1)
}

// createAssetWithContentCountUpdate creates an asset and increments the parent folder's ContentCount
func (d *DB) createAssetWithContentCountUpdate(ctx context.Context, asset assetDynamo, parentPath string, delta int) error {
	item, err := attributevalue.MarshalMap(asset)
	if err != nil {
		return assets.NewFailedToWriteError("failed to marshal asset", err)
	}

	// Build the condition for the new asset (must not exist)
	assetCond := expression.AttributeNotExists(expression.Name("PK"))
	assetExpr := exprMustBuild(expression.NewBuilder().WithCondition(assetCond))

	// Split parent path to get its PK/SK for ContentCount update
	var parentPKValue, parentSKValue string
	if parentPath == assets.RootPath {
		// Root folder has special path
		parentPKValue = pathPK("")
		parentSKValue = nameSK("/")
	} else {
		grandparentPath, parentName := splitFullPath(parentPath)
		parentPKValue = pathPK(grandparentPath)
		parentSKValue = nameSK(parentName)
	}

	// Build the update expression for parent ContentCount
	// Condition: parent must exist AND be a folder
	updateExpr := expression.Add(expression.Name("ContentCount"), expression.Value(delta))
	parentCond := expression.And(
		expression.AttributeExists(expression.Name("PK")),
		expression.Name("Type").Equal(expression.Value(assetTypeFolder)),
	)
	parentExpr := exprMustBuild(expression.NewBuilder().WithUpdate(updateExpr).WithCondition(parentCond))

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
						"PK": &types.AttributeValueMemberS{Value: parentPKValue},
						"SK": &types.AttributeValueMemberS{Value: parentSKValue},
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
func (d *DB) deleteAssetWithContentCountUpdate(ctx context.Context, fullPath, parentPath string, delta int) error {
	// Split asset full path to get parent path and name
	_, assetName := splitFullPath(fullPath)

	// Build the delete condition
	deleteCond := expression.AttributeExists(expression.Name("PK"))
	deleteExpr := exprMustBuild(expression.NewBuilder().WithCondition(deleteCond))

	// Split parent path to get its PK/SK for ContentCount update
	var parentPKValue, parentSKValue string
	if parentPath == assets.RootPath {
		// Root folder has special path
		parentPKValue = pathPK("")
		parentSKValue = nameSK("/")
	} else {
		grandparentPath, parentName := splitFullPath(parentPath)
		parentPKValue = pathPK(grandparentPath)
		parentSKValue = nameSK(parentName)
	}

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
						"PK": &types.AttributeValueMemberS{Value: pathPK(parentPath)},
						"SK": &types.AttributeValueMemberS{Value: nameSK(assetName)},
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
						"PK": &types.AttributeValueMemberS{Value: parentPKValue},
						"SK": &types.AttributeValueMemberS{Value: parentSKValue},
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
						return assets.NewNotFoundError(fmt.Sprintf("asset at path %q not found", fullPath), err)
					}
					// Parent doesn't exist - this shouldn't happen but handle gracefully
					return assets.NewParentFolderNotFoundError("parent folder not found")
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

// splitFullPath splits a full path to an asset into parent path and name.
// e.g., "/foo/bar.txt" -> ("/foo", "bar.txt")
// e.g., "/bar.txt" -> ("/", "bar.txt")
// e.g., "/" -> ("", "/") for root folder
func splitFullPath(fullPath string) (parentPath, name string) {
	// Handle root folder special case
	if fullPath == "/" {
		return "", "/"
	}

	dir, file := path.Split(fullPath)

	// path.Split includes trailing slash, remove it (but keep "/" as is)
	if dir != "/" {
		dir = strings.TrimSuffix(dir, "/")
	}

	return dir, file
}

// EnsureRootFolderExists creates the root folder if it doesn't exist.
// The root folder has a well-known ID (assets.RootFolderID), Path="" (no parent), and Name="/".
func (d *DB) EnsureRootFolderExists(ctx context.Context, createdBy string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

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
		return assets.NewFailedToWriteError("failed to marshal root folder", err)
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
			// Already exists
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return assets.NewTimeoutError("EnsureRootFolderExists timed out")
		}
		return assets.NewFailedToWriteError("failed to create root folder", err)
	}

	return nil
}
