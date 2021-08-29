package process

import (
	"math/big"
	"strings"

	"github.com/ElrondNetwork/elrond-go-logger/check"
	"github.com/ElrondNetwork/elrond-go/core"
	"github.com/ElrondNetwork/elrond-proxy-go/data"
)

const (
	esdtContractAddress   = "erd1qqqqqqqqqqqqqqqpqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqzllls8a5w6u"
	initialESDTSupplyFunc = "getTokenProperties"

	networkESDTSupplyPath = "/network/esdt/supply/"
)

type esdtSuppliesProcessor struct {
	baseProc    Processor
	scQueryProc SCQueryService
}

// NewESDTSuppliesProcessor will create a new instance of the ESDT supplies processor
func NewESDTSuppliesProcessor(baseProc Processor, scQueryProc SCQueryService) (*esdtSuppliesProcessor, error) {
	if check.IfNil(baseProc) {
		return nil, ErrNilCoreProcessor
	}
	if check.IfNil(scQueryProc) {
		return nil, ErrNilSCQueryService
	}

	return &esdtSuppliesProcessor{
		baseProc:    baseProc,
		scQueryProc: scQueryProc,
	}, nil
}

// GetESDTSupply will return the total supply for the provided token
func (esp *esdtSuppliesProcessor) GetESDTSupply(tokenIdentifier string) (*data.ESDTSupplyResponse, error) {
	totalSupply, err := esp.getSupplyFromShards(tokenIdentifier)
	if err != nil {
		return nil, err
	}

	res := &data.ESDTSupplyResponse{}
	if !isFungibleESDT(tokenIdentifier) {
		res.Data.Supply = totalSupply.String()
		return res, nil
	}

	initialSupply, err := esp.getInitialSupplyFromMeta(tokenIdentifier)
	if err != nil {
		return nil, err
	}

	totalSupply.Add(totalSupply, initialSupply)
	res.Data.Supply = totalSupply.String()

	return res, nil
}

func (esp *esdtSuppliesProcessor) getSupplyFromShards(tokenIdentifier string) (*big.Int, error) {
	totalSupply := big.NewInt(0)
	shardIDSs := esp.baseProc.GetShardIDs()
	for _, shardID := range shardIDSs {
		if shardID == core.MetachainShardId {
			continue
		}

		supply, err := esp.getShardSupply(tokenIdentifier, shardID)
		if err != nil {
			return nil, err
		}

		totalSupply.Add(totalSupply, supply)
	}

	return totalSupply, nil
}

func (esp *esdtSuppliesProcessor) getInitialSupplyFromMeta(token string) (*big.Int, error) {
	scQuery := &data.SCQuery{
		ScAddress: esdtContractAddress,
		FuncName:  initialESDTSupplyFunc,
		Arguments: [][]byte{[]byte(token)},
	}

	res, err := esp.scQueryProc.ExecuteQuery(scQuery)
	if err != nil {
		return nil, err
	}

	if len(res.ReturnData) < 4 {
		return big.NewInt(0), nil
	}

	supplyBytes := res.ReturnData[3]
	supplyBig, _ := big.NewInt(0).SetString(string(supplyBytes), 10)
	return supplyBig, nil
}

func (esp *esdtSuppliesProcessor) getShardSupply(token string, shardID uint32) (*big.Int, error) {
	shardObservers, errObs := esp.baseProc.GetObservers(shardID)
	if errObs != nil {
		return nil, errObs
	}

	apiPath := networkESDTSupplyPath + token
	for _, observer := range shardObservers {
		var responseEsdtSupply data.ESDTSupplyResponse

		_, errGet := esp.baseProc.CallGetRestEndPoint(observer.Address, apiPath, &responseEsdtSupply)
		if errGet != nil {
			log.Error("esdt supply request", "observer", observer.Address, "error", errGet.Error())
			continue
		}

		log.Info("esdt supply request", "shard ID", observer.ShardId, "observer", observer.Address)

		if responseEsdtSupply.Data.Supply == "" {
			return big.NewInt(0), nil
		}

		bigValue, _ := big.NewInt(0).SetString(responseEsdtSupply.Data.Supply, 10)
		return bigValue, nil

	}

	return nil, ErrSendingRequest
}

func isFungibleESDT(tokenIdentifier string) bool {
	splitToken := strings.Split(tokenIdentifier, "-")

	return len(splitToken) < 3
}
