package middleware

/*func TestContextMiddleware_OLD(t *testing.T) {
	es := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// set context that would usually be set in other application middleware
		userCtx := context.WithValue(r.Context(), "user", "jsmith@example.org")
		r = r.WithContext(userCtx)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(body) != privateQuery {
			t.Errorf("Expected %s, got %s", privateQuery, body)
		}
	}))
	defer es.Close()

	schemaProvider := handler.NewStaticSchemaProvider([]byte(publicSchema))

	ph := handler.NewHttpProxyHandler(es.URL, schemaProvider, &ContextMiddleware{})
	ts := httptest.NewServer(ph)
	defer ts.Close()

	t.Run("Test context middleware", func(t *testing.T) {
		r, err := http.NewRequest("POST", ts.URL, strings.NewReader(publicQuery))
		ctx := context.WithValue(context.Background(), "user", "jsmith@example.org")
		r = r.WithContext(ctx)
		if err != nil {
			t.Error(err)
		}
		client := http.DefaultClient
		_, err = client.Do(r)
		if err != nil {
			t.Error(err)
		}
	})
}*/
