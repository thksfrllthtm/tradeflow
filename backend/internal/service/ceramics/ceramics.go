package ceramics

// Ceramics module: name and price of ceramic tableware by mark, Chinese name and photo.
// Separate from estimates and the customs calculation.
//
// A base record is the company's name, mark and price. Product type, size and Chinese part are
// read from the name («Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘»).
//
// Order: exact mark in the base → size from the Chinese name / mark series → product type
// (same Chinese name in the base, otherwise AI choosing only from the base types) → the most
// frequent price of this type and size. Anything uncertain is NEED_REVIEW for the operator;
// nothing is saved without the operator's confirmation.

import (
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// Result statuses of a row.
const (
	CeramicsFoundByMark   = "FOUND_BY_MARK"
	CeramicsAuto          = "AUTO"
	CeramicsNeedReview    = "NEED_REVIEW"
	CeramicsNotDetermined = "NOT_DETERMINED"
)

// Statuses of the product type and the size.
const (
	CeramicsPartMatched    = "MATCHED"
	CeramicsPartAssumed    = "ASSUMED"
	CeramicsPartNeedReview = "NEED_REVIEW"
	CeramicsPartNotFound   = "NOT_FOUND"
	// The type is stored in the base without sizes.
	CeramicsPartNotNeeded = "NOT_NEEDED"
)

// Evidence sources shown to the operator.
const (
	SourceMarkExact        = "mark_exact"
	SourceChineseName      = "chinese_name"
	SourceChineseNumber    = "chinese_number"
	SourceChineseNameMatch = "chinese_name_match"
	SourceMarkPattern      = "mark_pattern"
	SourceRussianName      = "russian_name"
	SourceAI               = "ai"
	SourcePriceMode        = "price_mode"
)

const (
	maxCeramicsLookupMarks  = 5000
	maxCeramicsAnalyzeItems = 5
	maxCeramicsImportItems  = 5000
	maxCeramicsPhotoBytes   = 400 << 10
	maxCeramicsTextRunes    = 500
	ceramicsAnalyzeParallel = 3
)

type CeramicsService struct {
	repository *repository.Repository
	ai         *CeramicsAIClient
}

func NewCeramicsService(repo *repository.Repository, ai *CeramicsAIClient) *CeramicsService {
	return &CeramicsService{repository: repo, ai: ai}
}

// CeramicsProduct is a base record: name, mark and price, with the type, size and Chinese part
// read from the name.
type CeramicsProduct struct {
	ID          int64  `json:"id"`
	Mark        string `json:"mark"`
	Name        string `json:"name"`
	Price       string `json:"price"`
	ProductType string `json:"productType"`
	SizeValue   string `json:"sizeValue"`
	SizeUnit    string `json:"sizeUnit"`
	ChineseName string `json:"chineseName"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type CeramicsProductType struct {
	Name         string `json:"name"`
	ProductCount int64  `json:"productCount"`
}

// CeramicsProductInput is everything the base stores.
type CeramicsProductInput struct {
	Mark  string `json:"mark"`
	Name  string `json:"name"`
	Price string `json:"price"`
}

type CeramicsEvidence struct {
	// Field: "type", "size" or "price".
	Field  string `json:"field"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

type CeramicsSizeOption struct {
	Size   CeramicSize `json:"size"`
	Source string      `json:"source"`
	Detail string      `json:"detail"`
}

// CeramicsResolution is the answer for one mark.
type CeramicsResolution struct {
	Key            string                `json:"key"`
	Mark           string                `json:"mark"`
	MarkKey        string                `json:"markKey"`
	Status         string                `json:"status"`
	Product        *CeramicsProduct      `json:"product,omitempty"`
	ProductType    string                `json:"productType"`
	TypeStatus     string                `json:"typeStatus"`
	TypeCandidates []CeramicsAICandidate `json:"typeCandidates"`
	Size           *CeramicSize          `json:"size"`
	SizeStatus     string                `json:"sizeStatus"`
	SizeOptions    []CeramicsSizeOption  `json:"sizeOptions"`
	Price          CeramicsPriceResult   `json:"price"`
	// DisplayName: the base name for a found mark, otherwise the Russian part of the name
	// written like the other names of this type.
	DisplayName  string             `json:"displayName"`
	ChineseTerms []ChineseTerm      `json:"chineseTerms"`
	Evidence     []CeramicsEvidence `json:"evidence"`
	Warnings     []string           `json:"warnings"`
	AIUsed       bool               `json:"aiUsed"`
}

type CeramicsInputItem struct {
	Key         string `json:"key"`
	Mark        string `json:"mark"`
	ChineseName string `json:"chineseName"`
	RussianName string `json:"russianName"`
	Photo       string `json:"photo"`
}

type CeramicsImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type CeramicsAIStatus struct {
	Configured bool `json:"configured"`
	Vision     bool `json:"vision"`
}

func (s *CeramicsService) AIStatus() CeramicsAIStatus {
	return CeramicsAIStatus{Configured: s.ai.Configured(), Vision: s.ai.VisionEnabled()}
}
