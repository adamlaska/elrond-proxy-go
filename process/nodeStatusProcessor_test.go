package process

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ElrondNetwork/elrond-go/core"
	"github.com/ElrondNetwork/elrond-proxy-go/data"
	"github.com/ElrondNetwork/elrond-proxy-go/process/mock"
	"github.com/stretchr/testify/require"
)

func TestNewNodeStatusProcessor_NilBaseProcessor(t *testing.T) {
	t.Parallel()

	nodeStatusProc, err := NewNodeStatusProcessor(nil, &mock.GenericApiResponseCacherMock{}, time.Second, &mock.PubKeyConverterMock{})

	require.Equal(t, ErrNilCoreProcessor, err)
	require.Nil(t, nodeStatusProc)
}

func TestNewNodeStatusProcessor_NilCacher(t *testing.T) {
	t.Parallel()

	nodeStatusProc, err := NewNodeStatusProcessor(&mock.ProcessorStub{}, nil, time.Second, &mock.PubKeyConverterMock{})

	require.Equal(t, ErrNilEconomicMetricsCacher, err)
	require.Nil(t, nodeStatusProc)
}

func TestNewNodeStatusProcessor_InvalidCacheValidityDuration(t *testing.T) {
	t.Parallel()

	nodeStatusProc, err := NewNodeStatusProcessor(&mock.ProcessorStub{}, &mock.GenericApiResponseCacherMock{}, -1*time.Second, &mock.PubKeyConverterMock{})

	require.Equal(t, ErrInvalidCacheValidityDuration, err)
	require.Nil(t, nodeStatusProc)
}

func TestNodeStatusProcessor_GetConfigMetricsGetRestEndPointError(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetAllObserversCalled: func() ([]*data.NodeData, error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			return 0, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetNetworkConfigMetrics()
	require.Equal(t, ErrSendingRequest, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetConfigMetrics(t *testing.T) {
	t.Parallel()

	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetAllObserversCalled: func() ([]*data.NodeData, error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			localMap := map[string]interface{}{
				"key": 1,
			}
			genericResp := &data.GenericAPIResponse{Data: localMap}
			genRespBytes, _ := json.Marshal(genericResp)

			return 0, json.Unmarshal(genRespBytes, value)
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	genericResponse, err := nodeStatusProc.GetNetworkConfigMetrics()
	require.Nil(t, err)
	require.NotNil(t, genericResponse)

	map1, ok := genericResponse.Data.(map[string]interface{})
	require.True(t, ok)

	valueFromMap, ok := map1["key"]
	require.True(t, ok)
	require.Equal(t, 1, int(valueFromMap.(float64)))

}

func TestNodeStatusProcessor_GetNetworkMetricsGetObserversFailedShouldErr(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return nil, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetNetworkStatusMetrics(0)
	require.Equal(t, localErr, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetNetworkMetricsGetRestEndPointError(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			return 0, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetNetworkStatusMetrics(0)
	require.Equal(t, ErrSendingRequest, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetNetworkMetrics(t *testing.T) {
	t.Parallel()

	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			localMap := map[string]interface{}{
				"key": 1,
			}
			genericResp := &data.GenericAPIResponse{Data: localMap}
			genRespBytes, _ := json.Marshal(genericResp)

			return 0, json.Unmarshal(genRespBytes, value)
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	genericResponse, err := nodeStatusProc.GetNetworkStatusMetrics(0)
	require.Nil(t, err)
	require.NotNil(t, genericResponse)

	map1, ok := genericResponse.Data.(map[string]interface{})
	require.True(t, ok)

	valueFromMap, ok := map1["key"]
	require.True(t, ok)
	require.Equal(t, 1, int(valueFromMap.(float64)))
}

func TestNodeStatusProcessor_GetLatestBlockNonce(t *testing.T) {
	t.Parallel()

	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetAllObserversCalled: func() (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
				{Address: "address2", ShardId: core.MetachainShardId},
			}, nil
		},
		GetObserversCalled: func(shardId uint32) ([]*data.NodeData, error) {
			if shardId == 0 {
				return []*data.NodeData{
					{Address: "address1", ShardId: 0},
				}, nil
			} else {
				return []*data.NodeData{
					{Address: "address2", ShardId: core.MetachainShardId},
				}, nil
			}
		},

		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {

			var localMap map[string]interface{}
			if address == "address1" {
				localMap = map[string]interface{}{
					"metrics": map[string]interface{}{
						core.MetricCrossCheckBlockHeight: "meta 123",
					},
				}
			} else {
				localMap = map[string]interface{}{
					"metrics": map[string]interface{}{
						core.MetricNonce: 122,
					},
				}
			}

			genericResp := &data.GenericAPIResponse{Data: localMap}
			genRespBytes, _ := json.Marshal(genericResp)

			return 0, json.Unmarshal(genRespBytes, value)
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	nonce, err := nodeStatusProc.GetLatestFullySynchronizedHyperblockNonce()
	require.NoError(t, err)
	require.Equal(t, uint64(122), nonce)
}

func TestNodeStatusProcessor_GetAllIssuedEDTsGetObserversFailedShouldErr(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return nil, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetAllIssuedESDTs()
	require.Equal(t, localErr, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetAllIssuedESDTsGetRestEndPointError(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			return 0, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetAllIssuedESDTs()
	require.Equal(t, ErrSendingRequest, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetAllIssuedESDTs(t *testing.T) {
	t.Parallel()

	tokens := []string{"ESDT-5t6y7u", "NFT-9i8u7y-03"}
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			genericResp := &data.GenericAPIResponse{Data: tokens}
			genRespBytes, _ := json.Marshal(genericResp)

			return 0, json.Unmarshal(genRespBytes, value)
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	genericResponse, err := nodeStatusProc.GetAllIssuedESDTs()
	require.Nil(t, err)
	require.NotNil(t, genericResponse)

	slice, ok := genericResponse.Data.([]interface{})
	require.True(t, ok)

	for _, el := range slice {
		found := false
		for _, token := range tokens {
			if el.(string) == token {
				found = true
				break
			}
		}
		require.True(t, found)
	}
}

func TestNodeStatusProcessor_GetDelegatedInfoGetObserversFailedShouldErr(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return nil, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetDelegatedInfo()
	require.Equal(t, localErr, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetDelegatedInfoGetRestEndPointError(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			return 0, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetDelegatedInfo()
	require.Equal(t, ErrSendingRequest, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetDelegatedInfo(t *testing.T) {
	t.Parallel()

	expectedResp := &data.GenericAPIResponse{Data: "delegated info"}
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			genRespBytes, _ := json.Marshal(expectedResp)

			return 0, json.Unmarshal(genRespBytes, value)
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	actualResponse, err := nodeStatusProc.GetDelegatedInfo()
	require.Nil(t, err)
	require.Equal(t, expectedResp, actualResponse)
}

func TestNodeStatusProcessor_GetDirectStakedInfoGetObserversFailedShouldErr(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return nil, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetDirectStakedInfo()
	require.Equal(t, localErr, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetDirectStakedInfoGetRestEndPointError(t *testing.T) {
	t.Parallel()

	localErr := errors.New("local error")
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			return 0, localErr
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	status, err := nodeStatusProc.GetDirectStakedInfo()
	require.Equal(t, ErrSendingRequest, err)
	require.Nil(t, status)
}

func TestNodeStatusProcessor_GetDirectStakedInfo(t *testing.T) {
	t.Parallel()

	expectedResp := &data.GenericAPIResponse{Data: "direct staked info"}
	nodeStatusProc, _ := NewNodeStatusProcessor(&mock.ProcessorStub{
		GetObserversCalled: func(shardId uint32) (observers []*data.NodeData, err error) {
			return []*data.NodeData{
				{Address: "address1", ShardId: 0},
			}, nil
		},
		CallGetRestEndPointCalled: func(address string, path string, value interface{}) (int, error) {
			genRespBytes, _ := json.Marshal(expectedResp)

			return 0, json.Unmarshal(genRespBytes, value)
		},
	},
		&mock.GenericApiResponseCacherMock{},
		time.Nanosecond,
		&mock.PubKeyConverterMock{},
	)

	actualResponse, err := nodeStatusProc.GetDirectStakedInfo()
	require.Nil(t, err)
	require.Equal(t, expectedResp, actualResponse)
}
