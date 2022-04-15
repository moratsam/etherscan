package frontend

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"golang.org/x/xerrors"

	ss "github.com/moratsam/etherscan/scorestore"
)

//go:generate mockgen -package mocks -destination mocks/mocks.go github.com/moratsam/etherscan/frontend ScoreStoreAPI
//go:generate mockgen -package mocks -destination mocks/mock_indexer.go github.com/moratsam/etherscan/scorestore ScoreIterator

const (
	scoreStoreEndpoint      = "/"
	searchEndpoint     = "/search"
)

// ScoreStoreAPI defines a set of API methods for searching the score store
// for calculated gravitas scores.
type ScoreStoreAPI interface {
	Search(query ss.Query) (ss.ScoreIterator, error)
}

// Frontend implements the front-end component for the etherscan project.
type Frontend struct {
	cfg    Config
	router *mux.Router

	// A template executor hook which tests can override.
	tplExecutor func(tpl *template.Template, w io.Writer, data map[string]interface{}) error
}

// NewFrontend creates a new front-end instance with the specified config.
func NewFrontend(cfg Config) (*Frontend, error) {
	if err := cfg.validate(); err != nil {
		return nil, xerrors.Errorf("front-end service: config validation failed: %w", err)
	}

	f := &Frontend{
		router: mux.NewRouter(),
		cfg:    cfg,
		tplExecutor: func(tpl *template.Template, w io.Writer, data map[string]interface{}) error {
			return tpl.Execute(w, data)
		},
	}

	f.router.HandleFunc(scoreStoreEndpoint, f.renderScoreStorePage).Methods("GET")
	f.router.HandleFunc(searchEndpoint, f.renderSearchResults).Methods("GET")
	f.router.NotFoundHandler = http.HandlerFunc(f.render404Page)
	return f, nil
}


func (f *Frontend) Serve(ctx context.Context) error {
	l, err := net.Listen("tcp", f.cfg.ListenAddr)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()

	srv := &http.Server{
		Addr:    f.cfg.ListenAddr,
		Handler: f.router,
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	if err = srv.Serve(l); err == http.ErrServerClosed {
		// Ignore error when the server shuts down.
		err = nil
	}

	return err
}

func (f *Frontend) renderScoreStorePage(w http.ResponseWriter, _ *http.Request) {
	_ = f.tplExecutor(scoreStorePageTemplate, w, map[string]interface{}{
		"searchEndpoint":     searchEndpoint,
	})
}

func (f *Frontend) render404Page(w http.ResponseWriter, _ *http.Request) {
	_ = f.tplExecutor(msgPageTemplate, w, map[string]interface{}{
		"scoreStoreEndpoint":  scoreStoreEndpoint,
		"searchEndpoint": searchEndpoint,
		"messageTitle":   "Page not found",
		"messageContent": "Page not found.",
	})
}

func (f *Frontend) renderSearchErrorPage(w http.ResponseWriter, searchTerms string) {
	w.WriteHeader(http.StatusInternalServerError)
	_ = f.tplExecutor(msgPageTemplate, w, map[string]interface{}{
		"scoreStoreEndpoint":  scoreStoreEndpoint,
		"searchEndpoint": searchEndpoint,
		"searchTerms":    searchTerms,
		"messageTitle":   "Error",
		"messageContent": "An error occurred; please try again later.",
	})
}

func (f *Frontend) renderSearchResults(w http.ResponseWriter, r *http.Request) {
	searchTerms := r.URL.Query().Get("q")
	offset, _ := strconv.ParseUint(r.URL.Query().Get("offset"), 10, 64)

	scores, pagination, err := f.runQuery(searchTerms, offset)
	if err != nil {
		f.renderSearchErrorPage(w, searchTerms)
		return
	}

	// Render results page
	if err := f.tplExecutor(resultsPageTemplate, w, map[string]interface{}{
		"scoreStoreEndpoint":  scoreStoreEndpoint,
		"searchEndpoint": searchEndpoint,
		"searchTerms":    searchTerms,
		"pagination":     pagination,
		"results":        scores,
	}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (f *Frontend) runQuery(searchTerms string, offset uint64) ([]score, *paginationDetails, error) {
	if strings.HasPrefix(searchTerms, `"`) && strings.HasSuffix(searchTerms, `"`) {
		searchTerms = strings.Trim(searchTerms, `"`)
	}
	var query = ss.Query{Type: ss.QueryTypeScorer, Expression: searchTerms, Offset: offset}

	resultIt, err := f.cfg.ScoreStoreAPI.Search(query)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resultIt.Close() }()

	// Wrap each result in a score shim.
	scores := make([]score, 0, f.cfg.ResultsPerPage)
	for resCount := 0; resultIt.Next() && resCount < f.cfg.ResultsPerPage; resCount++ {
		next := resultIt.Score()
		scores = append(scores, score{score: next})
	}

	if err = resultIt.Error(); err != nil {
		return nil, nil, err
	}

	// Setup paginator and generate prev/next links
	pagination := &paginationDetails{
		From:  int(offset + 1),
		To:    int(offset) + len(scores),
		Total: int(resultIt.TotalCount()),
	}
	if offset > 0 {
		pagination.PrevLink = fmt.Sprintf("%s?q=%s", searchEndpoint, searchTerms)
		if prevOffset := int(offset) - f.cfg.ResultsPerPage; prevOffset > 0 {
			pagination.PrevLink += fmt.Sprintf("&offset=%d", prevOffset)
		}
	}
	if nextPageOffset := int(offset) + len(scores); nextPageOffset < pagination.Total {
		pagination.NextLink = fmt.Sprintf("%s?q=%s&offset=%d", searchEndpoint, searchTerms, nextPageOffset)
	}

	return scores, pagination, nil
}

// paginationDetails encapsulates the details for rendering a paginator component.
type paginationDetails struct {
	From     int
	To       int
	Total    int
	PrevLink string
	NextLink string
}

// mathcedDoc wraps an scoreStore.Score and provides convenience methods for
// rendering its contents in a search results view.
type score struct {
	score     *ss.Score
}

func (s *score) Wallet() string	{ return s.score.Wallet }
func (s *score) Value() string	{ return s.score.Value.String() }
