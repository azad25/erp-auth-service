package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"go.uber.org/zap"
)

// Client wraps the Elasticsearch client with additional functionality
type Client struct {
	es     *elasticsearch.Client
	logger *zap.Logger
}

// Config holds Elasticsearch configuration
type Config struct {
	Addresses []string `json:"addresses"`
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	APIKey    string   `json:"api_key"`
}

// NewClient creates a new Elasticsearch client
func NewClient(config Config, logger *zap.Logger) (*Client, error) {
	cfg := elasticsearch.Config{
		Addresses: config.Addresses,
		Username:  config.Username,
		Password:  config.Password,
		APIKey:    config.APIKey,
	}

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}

	client := &Client{
		es:     es,
		logger: logger,
	}

	// Test connection
	if err := client.ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to Elasticsearch: %w", err)
	}

	logger.Info("Elasticsearch client initialized successfully",
		zap.Strings("addresses", config.Addresses))

	return client, nil
}

// ping tests the connection to Elasticsearch
func (c *Client) ping(ctx context.Context) error {
	res, err := c.es.Info()
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch ping failed: %s", res.Status())
	}

	return nil
}

// IndexDocument indexes a document in Elasticsearch
func (c *Client) IndexDocument(ctx context.Context, index string, docID string, document interface{}) error {
	docBytes, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	req := esapi.IndexRequest{
		Index:      index,
		DocumentID: docID,
		Body:       bytes.NewReader(docBytes),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch indexing failed: %s", res.Status())
	}

	c.logger.Debug("Document indexed successfully",
		zap.String("index", index),
		zap.String("doc_id", docID))

	return nil
}

// SearchDocuments searches for documents in Elasticsearch
func (c *Client) SearchDocuments(ctx context.Context, index string, query map[string]interface{}) (*SearchResponse, error) {
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req := esapi.SearchRequest{
		Index: []string{index},
		Body:  bytes.NewReader(queryBytes),
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch search failed: %s", res.Status())
	}

	var searchResponse SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	return &searchResponse, nil
}

// CreateIndex creates an index with the specified mapping
func (c *Client) CreateIndex(ctx context.Context, index string, mapping map[string]interface{}) error {
	// Check if index already exists
	req := esapi.IndicesExistsRequest{
		Index: []string{index},
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	res.Body.Close()

	// Index already exists
	if res.StatusCode == 200 {
		c.logger.Debug("Index already exists", zap.String("index", index))
		return nil
	}

	// Create index
	mappingBytes, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal mapping: %w", err)
	}

	createReq := esapi.IndicesCreateRequest{
		Index: index,
		Body:  bytes.NewReader(mappingBytes),
	}

	createRes, err := createReq.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer createRes.Body.Close()

	if createRes.IsError() {
		return fmt.Errorf("elasticsearch index creation failed: %s", createRes.Status())
	}

	c.logger.Info("Index created successfully", zap.String("index", index))
	return nil
}

// BulkIndex performs bulk indexing of documents
func (c *Client) BulkIndex(ctx context.Context, index string, documents []BulkDocument) error {
	if len(documents) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, doc := range documents {
		// Index action
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": index,
				"_id":    doc.ID,
			},
		}
		actionBytes, _ := json.Marshal(action)
		buf.Write(actionBytes)
		buf.WriteByte('\n')

		// Document
		docBytes, _ := json.Marshal(doc.Source)
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	req := esapi.BulkRequest{
		Body:    &buf,
		Refresh: "true",
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to perform bulk index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch bulk indexing failed: %s", res.Status())
	}

	c.logger.Debug("Bulk indexing completed",
		zap.String("index", index),
		zap.Int("document_count", len(documents)))

	return nil
}

// SearchResponse represents an Elasticsearch search response
type SearchResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Hits     struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		MaxScore float64 `json:"max_score"`
		Hits     []struct {
			Index  string                 `json:"_index"`
			ID     string                 `json:"_id"`
			Score  float64                `json:"_score"`
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
}

// BulkDocument represents a document for bulk indexing
type BulkDocument struct {
	ID     string      `json:"id"`
	Source interface{} `json:"source"`
}

// Close closes the Elasticsearch client
func (c *Client) Close() error {
	// The go-elasticsearch client doesn't have a Close method
	// but we can log that we're shutting down
	c.logger.Info("Elasticsearch client shutting down")
	return nil
}
