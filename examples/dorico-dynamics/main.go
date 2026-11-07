package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	apix "github.com/Infra-Forge/infra-apix"
	echoadapter "github.com/Infra-Forge/infra-apix/echo"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/Infra-Forge/infra-apix/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/shopspring/decimal"
)

// Domain Models

type DocumentModel struct {
	ID            uuid.UUID  `json:"id"`
	FileName      string     `json:"file_name" validate:"required"`
	ContentType   string     `json:"content_type"`
	FileSize      int64      `json:"file_size"`
	ProcessStatus string     `json:"process_status"`
	ProcessedAt   *time.Time `json:"processed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	RetentionDate time.Time  `json:"retention_date"`
	DataSource    string     `json:"data_source"`
}

type TransactionModel struct {
	ID              uuid.UUID       `json:"id"`
	DocumentID      uuid.UUID       `json:"document_id"`
	TransactionDate time.Time       `json:"transaction_date"`
	Description     string          `json:"description" validate:"required"`
	Amount          decimal.Decimal `json:"amount"`
	TransactionType string          `json:"transaction_type"`
	CategoryID      *uuid.UUID      `json:"category_id,omitempty"`
	MerchantName    string          `json:"merchant_name,omitempty"`
	IsRecurring     bool            `json:"is_recurring"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type CategoryModel struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name" validate:"required"`
	Description string     `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	Color       string     `json:"color,omitempty"`
	Icon        string     `json:"icon,omitempty"`
	IsDefault   bool       `json:"is_default"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Request/Response DTOs

type CreateDocumentRequest struct {
	FileName    string `json:"file_name" validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
	FileSize    int64  `json:"file_size" validate:"required,min=1"`
	DataSource  string `json:"data_source" validate:"required,oneof=user_upload bank_import api_sync"`
}

type CreateTransactionRequest struct {
	DocumentID      uuid.UUID       `json:"document_id" validate:"required"`
	TransactionDate time.Time       `json:"transaction_date" validate:"required"`
	Description     string          `json:"description" validate:"required"`
	Amount          decimal.Decimal `json:"amount" validate:"required"`
	TransactionType string          `json:"transaction_type" validate:"required,oneof=income expense transfer"`
	CategoryID      *uuid.UUID      `json:"category_id,omitempty"`
	MerchantName    string          `json:"merchant_name,omitempty"`
}

type CreateCategoryRequest struct {
	Name        string     `json:"name" validate:"required"`
	Description string     `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	Color       string     `json:"color,omitempty"`
	Icon        string     `json:"icon,omitempty"`
}

type UpdateCategoryRequest struct {
	Name        *string    `json:"name,omitempty"`
	Description *string    `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	Color       *string    `json:"color,omitempty"`
	Icon        *string    `json:"icon,omitempty"`
}

type PaginationMeta struct {
	Page    int  `json:"page"`
	PerPage int  `json:"per_page"`
	Total   int  `json:"total"`
	Pages   int  `json:"pages"`
	HasNext bool `json:"has_next"`
	HasPrev bool `json:"has_prev"`
}

type DocumentListResponse struct {
	Data       []DocumentModel `json:"data"`
	Pagination PaginationMeta  `json:"pagination"`
}

type TransactionListResponse struct {
	Data       []TransactionModel `json:"data"`
	Pagination PaginationMeta     `json:"pagination"`
}

// Handlers (production-grade implementations)

func createDocument(ctx context.Context, req *CreateDocumentRequest) (DocumentModel, error) {
	now := time.Now()
	doc := DocumentModel{
		ID:            uuid.New(),
		FileName:      req.FileName,
		ContentType:   req.ContentType,
		FileSize:      req.FileSize,
		ProcessStatus: "pending",
		CreatedAt:     now,
		UpdatedAt:     now,
		RetentionDate: now.AddDate(7, 0, 0), // 7 years retention
		DataSource:    req.DataSource,
	}
	return doc, nil
}

func getDocument(ctx context.Context, _ *apix.NoBody) (DocumentModel, error) {
	// In production, this would fetch from database
	now := time.Now()
	return DocumentModel{
		ID:            uuid.New(),
		FileName:      "bank_statement_jan_2025.pdf",
		ContentType:   "application/pdf",
		FileSize:      1048576,
		ProcessStatus: "completed",
		ProcessedAt:   &now,
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now,
		RetentionDate: now.AddDate(7, 0, 0),
		DataSource:    "user_upload",
	}, nil
}

func listDocuments(ctx context.Context, _ *apix.NoBody) (DocumentListResponse, error) {
	// In production, this would query database with pagination
	now := time.Now()
	docs := []DocumentModel{
		{
			ID:            uuid.New(),
			FileName:      "statement_jan.pdf",
			ContentType:   "application/pdf",
			FileSize:      1048576,
			ProcessStatus: "completed",
			ProcessedAt:   &now,
			CreatedAt:     now.Add(-24 * time.Hour),
			UpdatedAt:     now,
			RetentionDate: now.AddDate(7, 0, 0),
			DataSource:    "user_upload",
		},
	}

	return DocumentListResponse{
		Data: docs,
		Pagination: PaginationMeta{
			Page:    1,
			PerPage: 20,
			Total:   1,
			Pages:   1,
			HasNext: false,
			HasPrev: false,
		},
	}, nil
}

func createTransaction(ctx context.Context, req *CreateTransactionRequest) (TransactionModel, error) {
	now := time.Now()
	return TransactionModel{
		ID:              uuid.New(),
		DocumentID:      req.DocumentID,
		TransactionDate: req.TransactionDate,
		Description:     req.Description,
		Amount:          req.Amount,
		TransactionType: req.TransactionType,
		CategoryID:      req.CategoryID,
		MerchantName:    req.MerchantName,
		IsRecurring:     false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func listTransactions(ctx context.Context, _ *apix.NoBody) (TransactionListResponse, error) {
	now := time.Now()
	amount, _ := decimal.NewFromString("156.75")

	txns := []TransactionModel{
		{
			ID:              uuid.New(),
			DocumentID:      uuid.New(),
			TransactionDate: now.Add(-24 * time.Hour),
			Description:     "Grocery shopping at Whole Foods",
			Amount:          amount,
			TransactionType: "expense",
			MerchantName:    "Whole Foods Market",
			IsRecurring:     false,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
	}

	return TransactionListResponse{
		Data: txns,
		Pagination: PaginationMeta{
			Page:    1,
			PerPage: 20,
			Total:   1,
			Pages:   1,
			HasNext: false,
			HasPrev: false,
		},
	}, nil
}

func createCategory(ctx context.Context, req *CreateCategoryRequest) (CategoryModel, error) {
	now := time.Now()
	return CategoryModel{
		ID:          uuid.New(),
		Name:        req.Name,
		Description: req.Description,
		ParentID:    req.ParentID,
		Color:       req.Color,
		Icon:        req.Icon,
		IsDefault:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func updateCategory(ctx context.Context, req *UpdateCategoryRequest) (CategoryModel, error) {
	now := time.Now()
	color := ""
	if req.Color != nil {
		color = *req.Color
	}
	icon := ""
	if req.Icon != nil {
		icon = *req.Icon
	}
	name := ""
	if req.Name != nil {
		name = *req.Name
	}
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	return CategoryModel{
		ID:          uuid.New(),
		Name:        name,
		Description: desc,
		ParentID:    req.ParentID,
		Color:       color,
		Icon:        icon,
		IsDefault:   false,
		CreatedAt:   now.Add(-7 * 24 * time.Hour),
		UpdatedAt:   now,
	}, nil
}

func deleteCategory(ctx context.Context, _ *apix.NoBody) (apix.NoBody, error) {
	// In production, this would delete from database
	return apix.NoBody{}, nil
}

func main() {
	// Reset registry for clean state
	apix.ResetRegistry()

	// Create Echo instance
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Create apix adapter
	adapter := echoadapter.New(e)

	// Register Document routes
	echoadapter.Post(adapter, "/api/documents", createDocument,
		apix.WithSummary("Upload financial document"),
		apix.WithDescription("Upload a new financial document for processing (bank statement, invoice, receipt)"),
		apix.WithTags("Documents"),
		apix.WithStandardErrors(),
		apix.WithNotFoundError("Document not found"),
	)

	echoadapter.Get(adapter, "/api/documents/:id", getDocument,
		apix.WithSummary("Get document by ID"),
		apix.WithDescription("Retrieve a specific financial document by its ID"),
		apix.WithTags("Documents"),
		apix.WithParameter(apix.Parameter{
			Name:        "id",
			In:          "path",
			Required:    true,
			SchemaType:  "string",
			Description: "Document UUID",
		}),
		apix.WithNotFoundError("Document not found"),
	)

	echoadapter.Get(adapter, "/api/documents", listDocuments,
		apix.WithSummary("List all documents"),
		apix.WithDescription("Retrieve a paginated list of all financial documents"),
		apix.WithTags("Documents"),
		apix.WithParameter(apix.Parameter{Name: "page", In: "query", SchemaType: "integer", Description: "Page number"}),
		apix.WithParameter(apix.Parameter{Name: "per_page", In: "query", SchemaType: "integer", Description: "Items per page"}),
	)

	// Register Transaction routes
	echoadapter.Post(adapter, "/api/transactions", createTransaction,
		apix.WithSummary("Create transaction"),
		apix.WithDescription("Create a new financial transaction"),
		apix.WithTags("Transactions"),
		apix.WithSecurity("BearerAuth", "transactions:write"),
		apix.WithStandardErrors(),
	)

	echoadapter.Get(adapter, "/api/transactions", listTransactions,
		apix.WithSummary("List transactions"),
		apix.WithDescription("Retrieve a paginated list of financial transactions"),
		apix.WithTags("Transactions"),
		apix.WithSecurity("BearerAuth", "transactions:read"),
		apix.WithParameter(apix.Parameter{Name: "page", In: "query", SchemaType: "integer"}),
		apix.WithParameter(apix.Parameter{Name: "per_page", In: "query", SchemaType: "integer"}),
		apix.WithParameter(apix.Parameter{Name: "category_id", In: "query", SchemaType: "string", Description: "Filter by category"}),
		apix.WithParameter(apix.Parameter{Name: "start_date", In: "query", SchemaType: "string", Description: "Filter by start date (ISO 8601)"}),
		apix.WithParameter(apix.Parameter{Name: "end_date", In: "query", SchemaType: "string", Description: "Filter by end date (ISO 8601)"}),
	)

	// Register Category routes (full CRUD)
	echoadapter.Post(adapter, "/api/categories", createCategory,
		apix.WithSummary("Create category"),
		apix.WithDescription("Create a new transaction category"),
		apix.WithTags("Categories"),
		apix.WithSecurity("BearerAuth", "categories:write"),
		apix.WithStandardErrors(),
		apix.WithConflictError("Category with this name already exists"),
	)

	echoadapter.Put(adapter, "/api/categories/:id", updateCategory,
		apix.WithSummary("Update category"),
		apix.WithDescription("Update an existing transaction category"),
		apix.WithTags("Categories"),
		apix.WithSecurity("BearerAuth", "categories:write"),
		apix.WithParameter(apix.Parameter{Name: "id", In: "path", Required: true, SchemaType: "string"}),
		apix.WithStandardErrors(),
		apix.WithNotFoundError("Category not found"),
	)

	echoadapter.Delete(adapter, "/api/categories/:id", deleteCategory,
		apix.WithSummary("Delete category"),
		apix.WithDescription("Delete a transaction category"),
		apix.WithTags("Categories"),
		apix.WithSecurity("BearerAuth", "categories:delete"),
		apix.WithParameter(apix.Parameter{Name: "id", In: "path", Required: true, SchemaType: "string"}),
		apix.WithNotFoundError("Category not found"),
		apix.WithConflictError("Cannot delete category with existing transactions"),
	)

	// Create runtime handler for OpenAPI spec
	handler, err := runtime.NewHandler(runtime.Config{
		Title:           "Dorico Dynamics Financial Analytics API",
		Version:         "1.0.0",
		Format:          "json",
		Servers:         []string{"http://localhost:8080"},
		EnableSwaggerUI: true,
		// Every scheme the routes name with apix.WithSecurity must be declared
		SecuritySchemes: openapi3.SecuritySchemes{
			"BearerAuth": &openapi3.SecuritySchemeRef{
				Value: openapi3.NewJWTSecurityScheme(),
			},
		},
		CustomizeBuilder: func(b *openapi.Builder) {
			// Add description
			b.Info.Description = "Production-grade financial analytics and document processing API built with apix"

			// Add contact info
			b.Info.Contact = &openapi3.Contact{
				Name: "Dorico Dynamics LDA",
				URL:  "https://doricod.co.mz",
			}

			// Add license
			b.Info.License = &openapi3.License{
				Name: "MIT",
				URL:  "https://opensource.org/licenses/MIT",
			}
		},
	})
	if err != nil {
		log.Fatalf("Failed to create runtime handler: %v", err)
	}

	// Register OpenAPI endpoints
	handler.RegisterEcho(e)

	// Health check endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "healthy",
			"version": "1.0.0",
		})
	})

	// Start server
	port := 8080
	log.Printf("Starting Dorico Dynamics example server on port %d", port)
	log.Printf("OpenAPI spec available at: http://localhost:%d/openapi.json", port)
	log.Printf("Swagger UI available at: http://localhost:%d/swagger", port)
	log.Printf("Health check available at: http://localhost:%d/health", port)

	if err := e.Start(fmt.Sprintf(":%d", port)); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
