package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/containous/traefik/v2/pkg/config/runtime"
	"github.com/containous/traefik/v2/pkg/config/static"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_HTTP(t *testing.T) {
	type expected struct {
		statusCode int
		nextPage   string
		jsonFile   string
	}

	testCases := []struct {
		desc     string
		path     string
		conf     runtime.Configuration
		expected expected
	}{
		{
			desc: "all routers, but no config",
			path: "/api/http/routers",
			conf: runtime.Configuration{},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "1",
				jsonFile:   "testdata/routers-empty.json",
			},
		},
		{
			desc: "all routers",
			path: "/api/http/routers",
			conf: runtime.Configuration{
				Routers: map[string]*runtime.RouterInfo{
					"test@myprovider": {
						Router: &dynamic.Router{
							EntryPoints: []string{"web"},
							Service:     "foo-service@myprovider",
							Rule:        "Host(`foo.bar.other`)",
							Middlewares: []string{"addPrefixTest", "auth"},
						},
					},
					"bar@myprovider": {
						Router: &dynamic.Router{
							EntryPoints: []string{"web"},
							Service:     "foo-service@myprovider",
							Rule:        "Host(`foo.bar`)",
							Middlewares: []string{"auth", "addPrefixTest@anotherprovider"},
						},
					},
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "1",
				jsonFile:   "testdata/routers.json",
			},
		},
		{
			desc: "all routers, pagination, 1 res per page, want page 2",
			path: "/api/http/routers?page=2&per_page=1",
			conf: runtime.Configuration{
				Routers: map[string]*runtime.RouterInfo{
					"bar@myprovider": {
						Router: &dynamic.Router{
							EntryPoints: []string{"web"},
							Service:     "foo-service@myprovider",
							Rule:        "Host(`foo.bar`)",
							Middlewares: []string{"auth", "addPrefixTest@anotherprovider"},
						},
					},
					"baz@myprovider": {
						Router: &dynamic.Router{
							EntryPoints: []string{"web"},
							Service:     "foo-service@myprovider",
							Rule:        "Host(`toto.bar`)",
						},
					},
					"test@myprovider": {
						Router: &dynamic.Router{
							EntryPoints: []string{"web"},
							Service:     "foo-service@myprovider",
							Rule:        "Host(`foo.bar.other`)",
							Middlewares: []string{"addPrefixTest", "auth"},
						},
					},
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "3",
				jsonFile:   "testdata/routers-page2.json",
			},
		},
		{
			desc: "all routers, pagination, 19 results overall, 7 res per page, want page 3",
			path: "/api/http/routers?page=3&per_page=7",
			conf: runtime.Configuration{
				Routers: generateHTTPRouters(19),
			},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "1",
				jsonFile:   "testdata/routers-many-lastpage.json",
			},
		},
		{
			desc: "all routers, pagination, 5 results overall, 10 res per page, want page 2",
			path: "/api/http/routers?page=2&per_page=10",
			conf: runtime.Configuration{
				Routers: generateHTTPRouters(5),
			},
			expected: expected{
				statusCode: http.StatusBadRequest,
			},
		},
		{
			desc: "all routers, pagination, 10 results overall, 10 res per page, want page 2",
			path: "/api/http/routers?page=2&per_page=10",
			conf: runtime.Configuration{
				Routers: generateHTTPRouters(10),
			},
			expected: expected{
				statusCode: http.StatusBadRequest,
			},
		},
		{
			desc: "one router by id",
			path: "/api/http/routers/bar@myprovider",
			conf: runtime.Configuration{
				Routers: map[string]*runtime.RouterInfo{
					"bar@myprovider": {
						Router: &dynamic.Router{
							EntryPoints: []string{"web"},
							Service:     "foo-service@myprovider",
							Rule:        "Host(`foo.bar`)",
							Middlewares: []string{"auth", "addPrefixTest@anotherprovider"},
						},
						Status: "enabled",
					},
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				jsonFile:   "testdata/router-bar.json",
			},
		},
		{
			desc: "one router by id, that does not exist",
			path: "/api/http/routers/foo@myprovider",
			conf: runtime.Configuration{
				Routers: map[string]*runtime.RouterInfo{
					"bar@myprovider": {
						Router: &dynamic.Router{
							EntryPoints: []string{"web"},
							Service:     "foo-service@myprovider",
							Rule:        "Host(`foo.bar`)",
							Middlewares: []string{"auth", "addPrefixTest@anotherprovider"},
						},
					},
				},
			},
			expected: expected{
				statusCode: http.StatusNotFound,
			},
		},
		{
			desc: "one router by id, but no config",
			path: "/api/http/routers/foo@myprovider",
			conf: runtime.Configuration{},
			expected: expected{
				statusCode: http.StatusNotFound,
			},
		},
		{
			desc: "all services, but no config",
			path: "/api/http/services",
			conf: runtime.Configuration{},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "1",
				jsonFile:   "testdata/services-empty.json",
			},
		},
		{
			desc: "all services",
			path: "/api/http/services",
			conf: runtime.Configuration{
				Services: map[string]*runtime.ServiceInfo{
					"bar@myprovider": func() *runtime.ServiceInfo {
						si := &runtime.ServiceInfo{
							Service: &dynamic.Service{
								LoadBalancer: &dynamic.LoadBalancerService{
									Servers: []dynamic.Server{
										{
											URL: "http://127.0.0.1",
										},
									},
								},
							},
							UsedBy: []string{"foo@myprovider", "test@myprovider"},
						}
						si.UpdateServerStatus("http://127.0.0.1", "UP")
						return si
					}(),
					"baz@myprovider": func() *runtime.ServiceInfo {
						si := &runtime.ServiceInfo{
							Service: &dynamic.Service{
								LoadBalancer: &dynamic.LoadBalancerService{
									Servers: []dynamic.Server{
										{
											URL: "http://127.0.0.2",
										},
									},
								},
							},
							UsedBy: []string{"foo@myprovider"},
						}
						si.UpdateServerStatus("http://127.0.0.2", "UP")
						return si
					}(),
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "1",
				jsonFile:   "testdata/services.json",
			},
		},
		{
			desc: "all services, 1 res per page, want page 2",
			path: "/api/http/services?page=2&per_page=1",
			conf: runtime.Configuration{
				Services: map[string]*runtime.ServiceInfo{
					"bar@myprovider": func() *runtime.ServiceInfo {
						si := &runtime.ServiceInfo{
							Service: &dynamic.Service{
								LoadBalancer: &dynamic.LoadBalancerService{
									Servers: []dynamic.Server{
										{
											URL: "http://127.0.0.1",
										},
									},
								},
							},
							UsedBy: []string{"foo@myprovider", "test@myprovider"},
						}
						si.UpdateServerStatus("http://127.0.0.1", "UP")
						return si
					}(),
					"baz@myprovider": func() *runtime.ServiceInfo {
						si := &runtime.ServiceInfo{
							Service: &dynamic.Service{
								LoadBalancer: &dynamic.LoadBalancerService{
									Servers: []dynamic.Server{
										{
											URL: "http://127.0.0.2",
										},
									},
								},
							},
							UsedBy: []string{"foo@myprovider"},
						}
						si.UpdateServerStatus("http://127.0.0.2", "UP")
						return si
					}(),
					"test@myprovider": func() *runtime.ServiceInfo {
						si := &runtime.ServiceInfo{
							Service: &dynamic.Service{
								LoadBalancer: &dynamic.LoadBalancerService{
									Servers: []dynamic.Server{
										{
											URL: "http://127.0.0.3",
										},
									},
								},
							},
							UsedBy: []string{"foo@myprovider", "test@myprovider"},
						}
						si.UpdateServerStatus("http://127.0.0.4", "UP")
						return si
					}(),
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "3",
				jsonFile:   "testdata/services-page2.json",
			},
		},
		{
			desc: "one service by id",
			path: "/api/http/services/bar@myprovider",
			conf: runtime.Configuration{
				Services: map[string]*runtime.ServiceInfo{
					"bar@myprovider": func() *runtime.ServiceInfo {
						si := &runtime.ServiceInfo{
							Service: &dynamic.Service{
								LoadBalancer: &dynamic.LoadBalancerService{
									Servers: []dynamic.Server{
										{
											URL: "http://127.0.0.1",
										},
									},
								},
							},
							UsedBy: []string{"foo@myprovider", "test@myprovider"},
						}
						si.UpdateServerStatus("http://127.0.0.1", "UP")
						return si
					}(),
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				jsonFile:   "testdata/service-bar.json",
			},
		},
		{
			desc: "one service by id, that does not exist",
			path: "/api/http/services/nono@myprovider",
			conf: runtime.Configuration{
				Services: map[string]*runtime.ServiceInfo{
					"bar@myprovider": func() *runtime.ServiceInfo {
						si := &runtime.ServiceInfo{
							Service: &dynamic.Service{
								LoadBalancer: &dynamic.LoadBalancerService{
									Servers: []dynamic.Server{
										{
											URL: "http://127.0.0.1",
										},
									},
								},
							},
							UsedBy: []string{"foo@myprovider", "test@myprovider"},
						}
						si.UpdateServerStatus("http://127.0.0.1", "UP")
						return si
					}(),
				},
			},
			expected: expected{
				statusCode: http.StatusNotFound,
			},
		},
		{
			desc: "one service by id, but no config",
			path: "/api/http/services/foo@myprovider",
			conf: runtime.Configuration{},
			expected: expected{
				statusCode: http.StatusNotFound,
			},
		},
		{
			desc: "all middlewares, but no config",
			path: "/api/http/middlewares",
			conf: runtime.Configuration{},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "1",
				jsonFile:   "testdata/middlewares-empty.json",
			},
		},
		{
			desc: "all middlewares",
			path: "/api/http/middlewares",
			conf: runtime.Configuration{
				Middlewares: map[string]*runtime.MiddlewareInfo{
					"auth@myprovider": {
						Middleware: &dynamic.Middleware{
							BasicAuth: &dynamic.BasicAuth{
								Users: []string{"admin:admin"},
							},
						},
						UsedBy: []string{"bar@myprovider", "test@myprovider"},
					},
					"addPrefixTest@myprovider": {
						Middleware: &dynamic.Middleware{
							AddPrefix: &dynamic.AddPrefix{
								Prefix: "/titi",
							},
						},
						UsedBy: []string{"test@myprovider"},
					},
					"addPrefixTest@anotherprovider": {
						Middleware: &dynamic.Middleware{
							AddPrefix: &dynamic.AddPrefix{
								Prefix: "/toto",
							},
						},
						UsedBy: []string{"bar@myprovider"},
					},
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "1",
				jsonFile:   "testdata/middlewares.json",
			},
		},
		{
			desc: "all middlewares, 1 res per page, want page 2",
			path: "/api/http/middlewares?page=2&per_page=1",
			conf: runtime.Configuration{
				Middlewares: map[string]*runtime.MiddlewareInfo{
					"auth@myprovider": {
						Middleware: &dynamic.Middleware{
							BasicAuth: &dynamic.BasicAuth{
								Users: []string{"admin:admin"},
							},
						},
						UsedBy: []string{"bar@myprovider", "test@myprovider"},
					},
					"addPrefixTest@myprovider": {
						Middleware: &dynamic.Middleware{
							AddPrefix: &dynamic.AddPrefix{
								Prefix: "/titi",
							},
						},
						UsedBy: []string{"test@myprovider"},
					},
					"addPrefixTest@anotherprovider": {
						Middleware: &dynamic.Middleware{
							AddPrefix: &dynamic.AddPrefix{
								Prefix: "/toto",
							},
						},
						UsedBy: []string{"bar@myprovider"},
					},
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				nextPage:   "3",
				jsonFile:   "testdata/middlewares-page2.json",
			},
		},
		{
			desc: "one middleware by id",
			path: "/api/http/middlewares/auth@myprovider",
			conf: runtime.Configuration{
				Middlewares: map[string]*runtime.MiddlewareInfo{
					"auth@myprovider": {
						Middleware: &dynamic.Middleware{
							BasicAuth: &dynamic.BasicAuth{
								Users: []string{"admin:admin"},
							},
						},
						UsedBy: []string{"bar@myprovider", "test@myprovider"},
					},
					"addPrefixTest@myprovider": {
						Middleware: &dynamic.Middleware{
							AddPrefix: &dynamic.AddPrefix{
								Prefix: "/titi",
							},
						},
						UsedBy: []string{"test@myprovider"},
					},
					"addPrefixTest@anotherprovider": {
						Middleware: &dynamic.Middleware{
							AddPrefix: &dynamic.AddPrefix{
								Prefix: "/toto",
							},
						},
						UsedBy: []string{"bar@myprovider"},
					},
				},
			},
			expected: expected{
				statusCode: http.StatusOK,
				jsonFile:   "testdata/middleware-auth.json",
			},
		},
		{
			desc: "one middleware by id, that does not exist",
			path: "/api/http/middlewares/foo@myprovider",
			conf: runtime.Configuration{
				Middlewares: map[string]*runtime.MiddlewareInfo{
					"auth@myprovider": {
						Middleware: &dynamic.Middleware{
							BasicAuth: &dynamic.BasicAuth{
								Users: []string{"admin:admin"},
							},
						},
						UsedBy: []string{"bar@myprovider", "test@myprovider"},
					},
				},
			},
			expected: expected{
				statusCode: http.StatusNotFound,
			},
		},
		{
			desc: "one middleware by id, but no config",
			path: "/api/http/middlewares/foo@myprovider",
			conf: runtime.Configuration{},
			expected: expected{
				statusCode: http.StatusNotFound,
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			rtConf := &test.conf
			// To lazily initialize the Statuses.
			rtConf.PopulateUsedBy()
			handler := New(static.Configuration{API: &static.API{}, Global: &static.Global{}}, rtConf)
			router := mux.NewRouter()
			handler.Append(router)

			server := httptest.NewServer(router)

			resp, err := http.DefaultClient.Get(server.URL + test.path)
			require.NoError(t, err)

			require.Equal(t, test.expected.statusCode, resp.StatusCode)

			assert.Equal(t, test.expected.nextPage, resp.Header.Get(nextPageHeader))

			if test.expected.jsonFile == "" {
				return
			}

			assert.Equal(t, resp.Header.Get("Content-Type"), "application/json")
			contents, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)

			err = resp.Body.Close()
			require.NoError(t, err)

			if *updateExpected {
				var results interface{}
				err := json.Unmarshal(contents, &results)
				require.NoError(t, err)

				newJSON, err := json.MarshalIndent(results, "", "\t")
				require.NoError(t, err)

				err = ioutil.WriteFile(test.expected.jsonFile, newJSON, 0644)
				require.NoError(t, err)
			}

			data, err := ioutil.ReadFile(test.expected.jsonFile)
			require.NoError(t, err)
			assert.JSONEq(t, string(data), string(contents))
		})
	}
}

func generateHTTPRouters(nbRouters int) map[string]*runtime.RouterInfo {
	routers := make(map[string]*runtime.RouterInfo, nbRouters)
	for i := 0; i < nbRouters; i++ {
		routers[fmt.Sprintf("bar%2d@myprovider", i)] = &runtime.RouterInfo{
			Router: &dynamic.Router{
				EntryPoints: []string{"web"},
				Service:     "foo-service@myprovider",
				Rule:        "Host(`foo.bar" + strconv.Itoa(i) + "`)",
			},
		}
	}
	return routers
}
