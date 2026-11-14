package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoStore ArticleStore 的 MongoDB 实现。
// collection articles：唯一索引 {item_id}，普通索引 {book_id}。
type mongoStore struct {
	coll   *mongo.Collection
	client *mongo.Client
}

// NewMongoStore 用现成的 *mongo.Collection 与 client 构建存储。
func NewMongoStore(coll *mongo.Collection, client *mongo.Client) *mongoStore {
	return &mongoStore{coll: coll, client: client}
}

// Close 断开底层 client。不在 ArticleStore 接口里（接口已冻结），
// confirm 阶段建了库却没人接管时做可选断言释放。
func (m *mongoStore) Close() error {
	if m.client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
	defer cancel()
	return m.client.Disconnect(ctx)
}

const mongoTimeout = 10 * time.Second

func ctx10() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), mongoTimeout)
}

// Init 幂等：collection 隐式创建；索引已存在不算错。
func (m *mongoStore) Init(ctx context.Context) error {
	iv := collView(m.coll)
	indexes := iv.Indexes()
	_, err := indexes.CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "item_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil && !isIndexOptionsConflict(err) {
		return fmt.Errorf("建 item_id 唯一索引: %w", err)
	}
	_, err = indexes.CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "book_id", Value: 1}},
		Options: options.Index().SetUnique(false),
	})
	if err != nil && !isIndexOptionsConflict(err) {
		return fmt.Errorf("建 book_id 索引: %w", err)
	}
	return nil
}

// isIndexOptionsConflict 同名同定义的索引已存在时 mongo 返回
// IndexOptionsConflict，按幂等处理。
func isIndexOptionsConflict(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "IndexOptionsConflict") ||
		strings.Contains(err.Error(), "IndexKeySpecsConflict"))
}

// Ping 用列表集合名探测连通性。
func (m *mongoStore) Ping(ctx context.Context) error {
	return m.coll.Database().Client().Ping(ctx, nil)
}

func (m *mongoStore) Get(ctx context.Context, bookID, itemID string) (*Article, error) {
	var row struct {
		ItemID    string    `bson:"item_id"`
		BookID    string    `bson:"book_id"`
		Title     string    `bson:"title"`
		Content   string    `bson:"content"`
		SizeBytes int64     `bson:"size_bytes"`
		CreatedAt time.Time `bson:"created_at"`
		UpdatedAt time.Time `bson:"updated_at"`
	}
	err := m.coll.FindOne(ctx, bson.M{"item_id": itemID, "book_id": bookID}).Decode(&row)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("查询文章: %w", err)
	}
	return &Article{
		ItemID:    row.ItemID,
		BookID:    row.BookID,
		Title:     row.Title,
		Content:   row.Content,
		SizeBytes: row.SizeBytes,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func (m *mongoStore) Put(ctx context.Context, a *Article) error {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	a.SizeBytes = int64(len(a.Content))
	filter := bson.M{"item_id": a.ItemID}
	// upsert：存在则更新业务字段并刷新 updated_at，保留 created_at。
	update := bson.M{"$set": bson.M{
		"book_id":    a.BookID,
		"title":      a.Title,
		"content":    a.Content,
		"size_bytes": a.SizeBytes,
		"updated_at": a.UpdatedAt,
	}}
	res, err := m.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("写入文章: %w", err)
	}
	if res.MatchedCount == 0 {
		_, err = m.coll.InsertOne(ctx, bson.M{
			"item_id":    a.ItemID,
			"book_id":    a.BookID,
			"title":      a.Title,
			"content":    a.Content,
			"size_bytes": a.SizeBytes,
			"created_at": a.CreatedAt,
			"updated_at": a.UpdatedAt,
		})
		if err != nil {
			return fmt.Errorf("插入文章: %w", err)
		}
	}
	return nil
}

func (m *mongoStore) Delete(ctx context.Context, bookID, itemID string) error {
	res, err := m.coll.DeleteOne(ctx, bson.M{"item_id": itemID, "book_id": bookID})
	if err != nil {
		return fmt.Errorf("删除文章: %w", err)
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *mongoStore) DeleteByBook(ctx context.Context, bookID string) (int64, error) {
	res, err := m.coll.DeleteMany(ctx, bson.M{"book_id": bookID})
	if err != nil {
		return 0, fmt.Errorf("按书删除: %w", err)
	}
	return res.DeletedCount, nil
}

func (m *mongoStore) DeleteAll(ctx context.Context) (int64, error) {
	res, err := m.coll.DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("清空缓存: %w", err)
	}
	return res.DeletedCount, nil
}

func (m *mongoStore) Count(ctx context.Context) (int64, error) {
	return m.coll.CountDocuments(ctx, bson.M{})
}

// SumSize 用 $group + $sum 聚合。
func (m *mongoStore) SumSize(ctx context.Context) (int64, error) {
	cur, err := m.coll.Aggregate(ctx, bson.A{
		bson.M{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$size_bytes"},
		}},
	})
	if err != nil {
		return 0, fmt.Errorf("聚合体积: %w", err)
	}
	defer cur.Close(ctx)
	var out struct {
		Total int64 `bson:"total"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&out); err != nil {
			return 0, err
		}
	}
	if err := cur.Err(); err != nil {
		return 0, err
	}
	return out.Total, nil
}

// collView 满足 Init 对集合视图的需要（保持签名可读）。
func collView(c *mongo.Collection) *mongo.Collection { return c }
