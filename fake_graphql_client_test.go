package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/machinebox/graphql"
)

type fakeGraphqlClient struct {
	responseFixturePaths []string
	reqIdx               int
}

func newFakeGraphqlClient(responseFixturePaths []string) *fakeGraphqlClient {
	return &fakeGraphqlClient{responseFixturePaths: responseFixturePaths}
}

func (g *fakeGraphqlClient) Run(_ context.Context, _ *graphql.Request, respPtr interface{}) error {
	responseFixture, err := os.Open(filepath.Join("testdata", g.responseFixturePaths[g.reqIdx]))
	if err != nil {
		return err
	}
	defer responseFixture.Close()
	var wrappedResponse map[string]interface{}
	if err := json.NewDecoder(responseFixture).Decode(&wrappedResponse); err != nil {
		return err
	}
	unwrappedSerialisedResp, err := json.Marshal(wrappedResponse["data"])
	if err != nil {
		return err
	}
	return json.Unmarshal(unwrappedSerialisedResp, respPtr)
}
