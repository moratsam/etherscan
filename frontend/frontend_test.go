package frontend

import (
	"fmt"
	"html/template"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/frontend/mocks"
	ss "github.com/moratsam/etherscan/scorestore"
)

var _ = gc.Suite(new(FrontendTestSuite))

type FrontendTestSuite struct {
}

func (s *FrontendTestSuite) TestSearch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockIt := s.mockScoreIterator(ctrl, 10)

	fe, mockScoreStore := s.setupFrontend(c, ctrl)
	mockScoreStore.EXPECT().Search(gomock.Any()).Return(mockIt, nil)

	fe.tplExecutor = func(_ *template.Template, _ io.Writer, data map[string]interface{}) error {
		pgDetails := data["pagination"].(*paginationDetails)
		c.Assert(pgDetails.From, gc.Equals, 1)
		c.Assert(pgDetails.To, gc.Equals, 2)
		c.Assert(pgDetails.Total, gc.Equals, 10)
		c.Assert(pgDetails.PrevLink, gc.Equals, "")
		c.Assert(pgDetails.NextLink, gc.Equals, "/search?q=KEYWORD&offset=2")
		return nil
	}

	req := httptest.NewRequest("GET", searchEndpoint+"?q=KEYWORD", nil)
	res := httptest.NewRecorder()
	fe.router.ServeHTTP(res, req)

	c.Assert(res.Code, gc.Equals, http.StatusOK)
}

func (s *FrontendTestSuite) TestPaginatedSearchOnSecondPage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockIt := s.mockScoreIterator(ctrl, 10)

	fe, mockScoreStore := s.setupFrontend(c, ctrl)
	mockScoreStore.EXPECT().Search(gomock.Any()).Return(mockIt, nil)

	fe.tplExecutor = func(_ *template.Template, _ io.Writer, data map[string]interface{}) error {
		pgDetails := data["pagination"].(*paginationDetails)
		c.Assert(pgDetails.From, gc.Equals, 3)
		c.Assert(pgDetails.To, gc.Equals, 4)
		c.Assert(pgDetails.Total, gc.Equals, 10)
		c.Assert(pgDetails.PrevLink, gc.Equals, "/search?q=KEYWORD")
		c.Assert(pgDetails.NextLink, gc.Equals, "/search?q=KEYWORD&offset=4")
		return nil
	}

	req := httptest.NewRequest("GET", searchEndpoint+"?q=KEYWORD&offset=2", nil)
	res := httptest.NewRecorder()
	fe.router.ServeHTTP(res, req)

	c.Assert(res.Code, gc.Equals, http.StatusOK)
}

func (s *FrontendTestSuite) TestPaginatedSearchOnThirdPage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockIt := s.mockScoreIterator(ctrl, 10)

	fe, mockScoreStore := s.setupFrontend(c, ctrl)
	mockScoreStore.EXPECT().Search(gomock.Any()).Return(mockIt, nil)

	fe.tplExecutor = func(_ *template.Template, _ io.Writer, data map[string]interface{}) error {
		pgDetails := data["pagination"].(*paginationDetails)
		c.Assert(pgDetails.From, gc.Equals, 5)
		c.Assert(pgDetails.To, gc.Equals, 6)
		c.Assert(pgDetails.Total, gc.Equals, 10)
		c.Assert(pgDetails.PrevLink, gc.Equals, "/search?q=KEYWORD&offset=2")
		c.Assert(pgDetails.NextLink, gc.Equals, "/search?q=KEYWORD&offset=6")
		return nil
	}

	req := httptest.NewRequest("GET", searchEndpoint+"?q=KEYWORD&offset=4", nil)
	res := httptest.NewRecorder()
	fe.router.ServeHTTP(res, req)

	c.Assert(res.Code, gc.Equals, http.StatusOK)
}

func (s *FrontendTestSuite) TestPaginatedSearchOnLastPage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockIt := s.mockScoreIterator(ctrl, 10)

	fe, mockScoreStore := s.setupFrontend(c, ctrl)
	mockScoreStore.EXPECT().Search(gomock.Any()).Return(mockIt, nil)

	fe.tplExecutor = func(_ *template.Template, _ io.Writer, data map[string]interface{}) error {
		pgDetails := data["pagination"].(*paginationDetails)
		c.Assert(pgDetails.From, gc.Equals, 9)
		c.Assert(pgDetails.To, gc.Equals, 10)
		c.Assert(pgDetails.Total, gc.Equals, 10)
		c.Assert(pgDetails.PrevLink, gc.Equals, "/search?q=KEYWORD&offset=6")
		c.Assert(pgDetails.NextLink, gc.Equals, "")
		return nil
	}

	req := httptest.NewRequest("GET", searchEndpoint+"?q=KEYWORD&offset=8", nil)
	res := httptest.NewRecorder()
	fe.router.ServeHTTP(res, req)

	c.Assert(res.Code, gc.Equals, http.StatusOK)
}

func (s *FrontendTestSuite) setupFrontend(c *gc.C, ctrl *gomock.Controller) (*Frontend, *mocks.MockScoreStoreAPI) {
	mockScoreStore := mocks.NewMockScoreStoreAPI(ctrl)

	fe, err := NewFrontend(Config{
		ScoreStoreAPI:		mockScoreStore,
		ListenAddr:			":0",
		ResultsPerPage:	2,
	})
	c.Assert(err, gc.IsNil)

	return fe, mockScoreStore
}

func (s *FrontendTestSuite) mockScoreIterator(ctrl *gomock.Controller, numResults int) *mocks.MockScoreIterator {
	it := mocks.NewMockScoreIterator(ctrl)
	it.EXPECT().TotalCount().Return(uint64(numResults))

	nextScore := 0
	it.EXPECT().Next().DoAndReturn(func() bool {
		nextScore++
		return nextScore < numResults
	}).MaxTimes(numResults)

	it.EXPECT().Score().DoAndReturn(func() *ss.Score {
		return &ss.Score{
			Wallet:	fmt.Sprintf("0x%d", nextScore),
			Value:	big.NewFloat(float64(nextScore)),
		}
	}).MaxTimes(numResults)

	it.EXPECT().Error().Return(nil)
	it.EXPECT().Close().Return(nil)
	return it
}
