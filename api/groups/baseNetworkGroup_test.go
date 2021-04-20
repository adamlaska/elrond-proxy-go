package groups_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ElrondNetwork/elrond-proxy-go/api/groups"
	"github.com/ElrondNetwork/elrond-proxy-go/api/mock"
	"github.com/ElrondNetwork/elrond-proxy-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const networkPath = "/network"

type metricsResponse struct {
	GeneralResponse
	Data map[string]interface{} `json:"data"`
}

func TestNewNetworkGroup_WrongFacadeShouldErr(t *testing.T) {
	wrongFacade := &mock.WrongFacade{}
	group, err := groups.NewNetworkGroup(wrongFacade)
	require.Nil(t, group)
	require.Equal(t, groups.ErrWrongTypeAssertion, err)
}

func TestGetNetworkStatusData_NoShardProvidedShouldErr(t *testing.T) {
	t.Parallel()

	facade := &mock.Facade{}

	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/status", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	statusRsp := metricsResponse{}
	loadResponse(resp.Body, &statusRsp)

	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestGetNetworkStatusData_FacadeFailsShouldErr(t *testing.T) {
	t.Parallel()

	facade := &mock.Facade{
		GetNetworkMetricsHandler: func(_ uint32) (*data.GenericAPIResponse, error) {
			return nil, errors.New("bad request")
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/status/0", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestGetNetworkStatusData_ShouldWork(t *testing.T) {
	t.Parallel()

	respMap := make(map[string]interface{})
	respMap["1"] = "2"
	respMap["2"] = "3"
	facade := &mock.Facade{
		GetNetworkMetricsHandler: func(_ uint32) (*data.GenericAPIResponse, error) {
			return &data.GenericAPIResponse{
				Data: respMap,
			}, nil
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/status/0", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result metricsResponse
	loadResponse(resp.Body, &result)

	assert.Equal(t, respMap, result.Data)
}

func TestGetNetworkConfigData_BadRequestShouldErr(t *testing.T) {
	t.Parallel()

	facade := &mock.Facade{
		GetConfigMetricsHandler: func() (*data.GenericAPIResponse, error) {
			return nil, errors.New("bad request")
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/config", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestGetNetworkConfigData_FacadeErrShouldErr(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("expected error")
	facade := &mock.Facade{
		GetConfigMetricsHandler: func() (*data.GenericAPIResponse, error) {
			return nil, expectedErr
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/config", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusInternalServerError, resp.Code)

	var result metricsResponse
	loadResponse(resp.Body, &result)

	assert.Equal(t, expectedErr.Error(), result.Error)
}

func TestGetNetworkConfigData_OkRequestShouldWork(t *testing.T) {
	t.Parallel()

	key := "erd_min_gas_limit"
	value := float64(37)
	facade := &mock.Facade{
		GetConfigMetricsHandler: func() (*data.GenericAPIResponse, error) {
			return &data.GenericAPIResponse{
				Data: map[string]interface{}{
					key: value,
				},
				Error: "",
			}, nil
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/config", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result metricsResponse
	loadResponse(resp.Body, &result)

	res, ok := result.Data[key]
	assert.True(t, ok)
	assert.Equal(t, value, res)
}

func TestGetEconomicsData_ShouldErr(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("internal error")
	facade := &mock.Facade{
		GetEconomicsDataMetricsHandler: func() (*data.GenericAPIResponse, error) {
			return nil, expectedErr
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/economics", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	ecDataResp := data.GenericAPIResponse{}
	loadResponse(resp.Body, &ecDataResp)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Equal(t, expectedErr.Error(), ecDataResp.Error)
}

func TestGetEconomicsData_ShouldWork(t *testing.T) {
	t.Parallel()

	expectedResp := data.GenericAPIResponse{Data: map[string]interface{}{"erd_total_supply": "12345"}}
	facade := &mock.Facade{
		GetEconomicsDataMetricsHandler: func() (*data.GenericAPIResponse, error) {
			return &expectedResp, nil
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/economics", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	ecDataResp := data.GenericAPIResponse{}
	loadResponse(resp.Body, &ecDataResp)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, expectedResp, ecDataResp)
	assert.Equal(t, expectedResp.Data, ecDataResp.Data) //extra safe
}

func TestGetAllIssuedESDTs_ShouldErr(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("internal error")
	facade := &mock.Facade{
		GetAllIssuedESDTsHandler: func() (*data.GenericAPIResponse, error) {
			return nil, expectedErr
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/esdts", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	allIssuedEsdts := data.GenericAPIResponse{}
	loadResponse(resp.Body, &allIssuedEsdts)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Equal(t, expectedErr.Error(), allIssuedEsdts.Error)
}

func TestGetAllIssuedESDTs_ShouldWork(t *testing.T) {
	t.Parallel()

	expectedResp := data.GenericAPIResponse{Data: []string{"ESDT-1w2e3e", "NFT-1q2w3e-01"}}
	facade := &mock.Facade{
		GetAllIssuedESDTsHandler: func() (*data.GenericAPIResponse, error) {
			return &expectedResp, nil
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/esdts", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	allIssuedESDTs := data.GenericAPIResponse{}
	loadResponse(resp.Body, &allIssuedESDTs)

	assert.Equal(t, http.StatusOK, resp.Code)

	for _, resp := range allIssuedESDTs.Data.([]interface{}) {
		respStr := resp.(string)
		found := false
		for _, exp := range expectedResp.Data.([]string) {
			if respStr == exp {
				found = true
				break
			}
		}

		assert.True(t, found, fmt.Sprintf("token %s not found", respStr))
	}
}

func TestGetEnableEpochsMetrics_FacadeErrShouldErr(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("expected err")
	facade := &mock.Facade{
		GetEnableEpochsMetricsHandler: func() (*data.GenericAPIResponse, error) {
			return nil, expectedErr
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/enable-epochs", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusInternalServerError, resp.Code)

	var result metricsResponse
	loadResponse(resp.Body, &result)

	assert.Equal(t, expectedErr.Error(), result.Error)
}

func TestGetEnableEpochsMetrics_BadRequestShouldErr(t *testing.T) {
	t.Parallel()

	facade := &mock.Facade{
		GetEnableEpochsMetricsHandler: func() (*data.GenericAPIResponse, error) {
			return nil, errors.New("bad request")
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/enable-epochs", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestGetEnableEpochsMetrics_OkRequestShouldWork(t *testing.T) {
	t.Parallel()

	key := "smart_contract_deploy"
	value := float64(4)
	facade := &mock.Facade{
		GetEnableEpochsMetricsHandler: func() (*data.GenericAPIResponse, error) {
			return &data.GenericAPIResponse{
				Data: map[string]interface{}{
					key: value,
				},
				Error: "",
			}, nil
		},
	}
	networkGroup, err := groups.NewNetworkGroup(facade)
	require.NoError(t, err)
	ws := startProxyServer(networkGroup, networkPath)

	req, _ := http.NewRequest("GET", "/network/enable-epochs", nil)
	resp := httptest.NewRecorder()
	ws.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result metricsResponse
	loadResponse(resp.Body, &result)

	res, ok := result.Data[key]
	assert.True(t, ok)
	assert.Equal(t, value, res)
}
