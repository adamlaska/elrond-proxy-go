package process

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"

	"github.com/ElrondNetwork/elrond-go-core/core"
	"github.com/ElrondNetwork/elrond-go-core/core/check"
	"github.com/ElrondNetwork/elrond-go-core/data/transaction"
	"github.com/ElrondNetwork/elrond-go-core/hashing"
	"github.com/ElrondNetwork/elrond-go-core/marshal"
	"github.com/ElrondNetwork/elrond-proxy-go/api/errors"
	"github.com/ElrondNetwork/elrond-proxy-go/data"
)

// TransactionPath defines the transaction group path of the node
const TransactionPath = "/transaction/"

// TransactionsPoolPath defines the transactions pool path of the node
const TransactionsPoolPath = "/transaction/pool"

// TransactionSendPath defines the single transaction send path of the node
const TransactionSendPath = "/transaction/send"

// TransactionSimulatePath defines single transaction simulate path of the node
const TransactionSimulatePath = "/transaction/simulate"

// MultipleTransactionsPath defines the multiple transactions send path of the node
const MultipleTransactionsPath = "/transaction/send-multiple"

// UnknownStatusTx defines the response that should be received from an observer when transaction status is unknown
const UnknownStatusTx = "unknown"

const (
	withResultsParam    = "?withResults=true"
	checkSignatureFalse = "?checkSignature=false"
	bySenderParam       = "&by-sender="
	fieldsParam         = "?fields="
	lastNonceParam      = "?last-nonce=true"
	nonceGapsParam      = "?nonce-gaps=true"
)

type requestType int

const (
	requestTypeObservers        requestType = 0
	requestTypeFullHistoryNodes requestType = 1
)

type erdTransaction struct {
	Nonce     uint64 `json:"nonce"`
	Value     string `json:"value"`
	RcvAddr   string `json:"receiver"`
	SndAddr   string `json:"sender"`
	GasPrice  uint64 `json:"gasPrice,omitempty"`
	GasLimit  uint64 `json:"gasLimit,omitempty"`
	Data      []byte `json:"data,omitempty"`
	Signature string `json:"signature,omitempty"`
	ChainID   string `json:"chainID"`
	Version   uint32 `json:"version"`
}

// TransactionProcessor is able to process transaction requests
type TransactionProcessor struct {
	proc                         Processor
	pubKeyConverter              core.PubkeyConverter
	hasher                       hashing.Hasher
	marshalizer                  marshal.Marshalizer
	newTxCostProcessor           func() (TransactionCostHandler, error)
	mergeLogsHandler             LogsMergerHandler
	shouldAllowEntireTxPoolFetch bool
}

// NewTransactionProcessor creates a new instance of TransactionProcessor
func NewTransactionProcessor(
	proc Processor,
	pubKeyConverter core.PubkeyConverter,
	hasher hashing.Hasher,
	marshalizer marshal.Marshalizer,
	newTxCostProcessor func() (TransactionCostHandler, error),
	logsMerger LogsMergerHandler,
	allowEntireTxPoolFetch bool,
) (*TransactionProcessor, error) {
	if check.IfNil(proc) {
		return nil, ErrNilCoreProcessor
	}
	if check.IfNil(pubKeyConverter) {
		return nil, ErrNilPubKeyConverter
	}
	if check.IfNil(hasher) {
		return nil, ErrNilHasher
	}
	if check.IfNil(marshalizer) {
		return nil, ErrNilMarshalizer
	}
	if newTxCostProcessor == nil {
		return nil, ErrNilNewTxCostHandlerFunc
	}
	if check.IfNil(logsMerger) {
		return nil, ErrNilLogsMerger
	}

	return &TransactionProcessor{
		proc:                         proc,
		pubKeyConverter:              pubKeyConverter,
		hasher:                       hasher,
		marshalizer:                  marshalizer,
		newTxCostProcessor:           newTxCostProcessor,
		mergeLogsHandler:             logsMerger,
		shouldAllowEntireTxPoolFetch: allowEntireTxPoolFetch,
	}, nil
}

// SendTransaction relays the post request by sending the request to the right observer and replies back the answer
func (tp *TransactionProcessor) SendTransaction(tx *data.Transaction) (int, string, error) {
	err := tp.checkTransactionFields(tx)
	if err != nil {
		return http.StatusBadRequest, "", err
	}

	senderBuff, err := tp.pubKeyConverter.Decode(tx.Sender)
	if err != nil {
		return http.StatusBadRequest, "", err
	}

	shardID, err := tp.proc.ComputeShardId(senderBuff)
	if err != nil {
		return http.StatusInternalServerError, "", err
	}

	observers, err := tp.proc.GetObservers(shardID)
	if err != nil {
		return http.StatusInternalServerError, "", err
	}

	for _, observer := range observers {
		txResponse := &data.ResponseTransaction{}

		respCode, err := tp.proc.CallPostRestEndPoint(observer.Address, TransactionSendPath, tx, txResponse)
		if respCode == http.StatusOK && err == nil {
			log.Info(fmt.Sprintf("Transaction sent successfully to observer %v from shard %v, received tx hash %s",
				observer.Address,
				shardID,
				txResponse.Data.TxHash,
			))
			return respCode, txResponse.Data.TxHash, nil
		}

		// if observer was down (or didn't respond in time), skip to the next one
		if respCode == http.StatusNotFound || respCode == http.StatusRequestTimeout {
			log.LogIfError(err)
			continue
		}

		// if the request was bad, return the error message
		return respCode, "", err
	}

	return http.StatusInternalServerError, "", ErrSendingRequest
}

// SimulateTransaction relays the post request by sending the request to the right observer and replies back the answer
func (tp *TransactionProcessor) SimulateTransaction(tx *data.Transaction, checkSignature bool) (*data.GenericAPIResponse, error) {
	err := tp.checkTransactionFields(tx)
	if err != nil {
		return nil, err
	}

	senderBuff, err := tp.pubKeyConverter.Decode(tx.Sender)
	if err != nil {
		return nil, err
	}

	senderShardID, err := tp.proc.ComputeShardId(senderBuff)
	if err != nil {
		return nil, err
	}

	observers, err := tp.proc.GetObservers(senderShardID)
	if err != nil {
		return nil, err
	}

	response, err := tp.simulateTransaction(observers, tx, checkSignature)
	if err != nil {
		return nil, fmt.Errorf("%w while trying to simulate on sender shard (shard %d)", err, senderShardID)
	}

	receiverBuff, err := tp.pubKeyConverter.Decode(tx.Receiver)
	if err != nil {
		return nil, err
	}

	receiverShardID, err := tp.proc.ComputeShardId(receiverBuff)
	if err != nil {
		return nil, err
	}

	if senderShardID == receiverShardID {
		return &data.GenericAPIResponse{
			Data:  response.Data,
			Error: response.Error,
			Code:  response.Code,
		}, nil
	}

	observersForReceiverShard, err := tp.proc.GetObservers(receiverShardID)
	if err != nil {
		return nil, err
	}

	responseFromReceiverShard, err := tp.simulateTransaction(observersForReceiverShard, tx, checkSignature)
	if err != nil {
		return nil, fmt.Errorf("%w while trying to simulate on receiver shard (shard %d)", err, receiverShardID)
	}

	simulationResult := data.ResponseTransactionSimulationCrossShard{}
	simulationResult.Data.Result = map[string]data.TransactionSimulationResults{
		"senderShard":   response.Data.Result,
		"receiverShard": responseFromReceiverShard.Data.Result,
	}

	return &data.GenericAPIResponse{
		Data:  simulationResult.Data,
		Error: "",
		Code:  data.ReturnCodeSuccess,
	}, nil
}

func (tp *TransactionProcessor) simulateTransaction(
	observers []*data.NodeData,
	tx *data.Transaction,
	checkSignature bool,
) (*data.ResponseTransactionSimulation, error) {
	txSimulatePath := TransactionSimulatePath
	if !checkSignature {
		txSimulatePath += checkSignatureFalse
	}

	for _, observer := range observers {
		txResponse := &data.ResponseTransactionSimulation{}

		respCode, err := tp.proc.CallPostRestEndPoint(observer.Address, txSimulatePath, tx, txResponse)
		if respCode == http.StatusOK && err == nil {
			log.Info(fmt.Sprintf("Transaction simulation sent successfully to observer %v from shard %v, received tx hash %s",
				observer.Address,
				observer.ShardId,
				txResponse.Data.Result.Hash,
			))
			return txResponse, nil
		}

		// if observer was down (or didn't respond in time), skip to the next one
		if respCode == http.StatusNotFound || respCode == http.StatusRequestTimeout {
			log.LogIfError(err)
			continue
		}

		// if the request was bad, return the error message
		return nil, err
	}

	return nil, ErrSendingRequest
}

// SendMultipleTransactions relays the post request by sending the request to the first available observer and replies back the answer
func (tp *TransactionProcessor) SendMultipleTransactions(txs []*data.Transaction) (
	data.MultipleTransactionsResponseData, error,
) {
	// TODO: Analyze and improve the robustness of this function. Currently, an error within `GetObservers`
	// breaks the function and returns nothing (but an error) even if some transactions were actually sent, successfully.

	totalTxsSent := uint64(0)
	txsToSend := make([]*data.Transaction, 0)
	for i := 0; i < len(txs); i++ {
		currentTx := txs[i]
		err := tp.checkTransactionFields(currentTx)
		if err != nil {
			log.Warn("invalid tx received",
				"sender", currentTx.Sender,
				"receiver", currentTx.Receiver,
				"error", err)
			continue
		}
		txsToSend = append(txsToSend, currentTx)
	}
	if len(txsToSend) == 0 {
		return data.MultipleTransactionsResponseData{}, ErrNoValidTransactionToSend
	}

	txsHashes := make(map[int]string)
	txsByShardID := tp.groupTxsByShard(txsToSend)
	for shardID, groupOfTxs := range txsByShardID {
		observersInShard, err := tp.proc.GetObservers(shardID)
		if err != nil {
			return data.MultipleTransactionsResponseData{}, ErrMissingObserver
		}

		for _, observer := range observersInShard {
			txResponse := &data.ResponseMultipleTransactions{}
			respCode, err := tp.proc.CallPostRestEndPoint(observer.Address, MultipleTransactionsPath, groupOfTxs, txResponse)
			if respCode == http.StatusOK && err == nil {
				log.Info("transactions sent",
					"observer", observer.Address,
					"shard ID", shardID,
					"total processed", txResponse.Data.NumOfTxs,
				)
				totalTxsSent += txResponse.Data.NumOfTxs

				for key, hash := range txResponse.Data.TxsHashes {
					txsHashes[groupOfTxs[key].Index] = hash
				}

				break
			}

			log.LogIfError(err)
		}
	}

	return data.MultipleTransactionsResponseData{
		NumOfTxs:  totalTxsSent,
		TxsHashes: txsHashes,
	}, nil
}

// TransactionCostRequest should return how many gas units a transaction will cost
func (tp *TransactionProcessor) TransactionCostRequest(tx *data.Transaction) (*data.TxCostResponseData, error) {
	err := tp.checkTransactionFields(tx)
	if err != nil {
		return nil, err
	}

	newTxCostProcessor, err := tp.newTxCostProcessor()
	if err != nil {
		return nil, err
	}

	return newTxCostProcessor.ResolveCostRequest(tx)
}

// GetTransaction should return a transaction from observer
func (tp *TransactionProcessor) GetTransaction(txHash string, withResults bool) (*transaction.ApiTransactionResult, error) {
	tx, err := tp.getTxFromObservers(txHash, requestTypeFullHistoryNodes, withResults)
	if err != nil {
		return nil, err
	}

	tx.HyperblockNonce = tx.NotarizedAtDestinationInMetaNonce
	tx.HyperblockHash = tx.NotarizedAtDestinationInMetaHash
	return tx, nil
}

// GetTransactionByHashAndSenderAddress returns a transaction
func (tp *TransactionProcessor) GetTransactionByHashAndSenderAddress(
	txHash string,
	sndAddr string,
	withEvents bool,
) (*transaction.ApiTransactionResult, int, error) {
	tx, err := tp.getTxWithSenderAddr(txHash, sndAddr, withEvents)
	if err != nil {
		return nil, http.StatusNotFound, err
	}

	return tx, http.StatusOK, nil
}

func (tp *TransactionProcessor) getShardByAddress(address string) (uint32, error) {
	var shardID uint32
	if metachainIDStr := fmt.Sprintf("%d", core.MetachainShardId); address != metachainIDStr {
		senderBuff, err := tp.pubKeyConverter.Decode(address)
		if err != nil {
			return 0, err
		}

		shardID, err = tp.proc.ComputeShardId(senderBuff)
		if err != nil {
			return 0, err
		}
	} else {
		shardID = core.MetachainShardId
	}

	return shardID, nil
}

// GetTransactionStatus returns the status of a transaction
func (tp *TransactionProcessor) GetTransactionStatus(txHash string, sender string) (string, error) {
	if sender != "" {
		tx, err := tp.getTxWithSenderAddr(txHash, sender, false)
		if err != nil {
			return UnknownStatusTx, err
		}

		return string(tx.Status), nil
	}

	// get status of transaction from random observers
	tx, err := tp.getTxFromObservers(txHash, requestTypeObservers, false)
	if err != nil {
		return UnknownStatusTx, errors.ErrTransactionNotFound
	}

	return string(tx.Status), nil
}

func (tp *TransactionProcessor) getTxFromObservers(txHash string, reqType requestType, withResults bool) (*transaction.ApiTransactionResult, error) {
	observersShardIDs := tp.proc.GetShardIDs()
	for _, observerShardID := range observersShardIDs {
		nodesInShard, err := tp.getNodesInShard(observerShardID, reqType)
		if err != nil {
			return nil, err
		}

		var getTxResponse *data.GetTransactionResponse
		var withHttpError bool
		var ok bool
		for _, observerInShard := range nodesInShard {
			getTxResponse, ok, withHttpError = tp.getTxFromObserver(observerInShard, txHash, withResults)
			if !withHttpError {
				break
			}
		}

		if !ok || getTxResponse == nil {
			continue
		}

		sndShardID, err := tp.getShardByAddress(getTxResponse.Data.Transaction.Sender)
		if err != nil {
			log.Warn("cannot compute shard ID from sender address",
				"sender address", getTxResponse.Data.Transaction.Sender,
				"error", err.Error())
		}

		rcvShardID, err := tp.getShardByAddress(getTxResponse.Data.Transaction.Receiver)
		if err != nil {
			log.Warn("cannot compute shard ID from receiver address",
				"receiver address", getTxResponse.Data.Transaction.Receiver,
				"error", err.Error())
		}

		isIntraShard := sndShardID == rcvShardID
		observerIsInDestShard := rcvShardID == observerShardID
		if isIntraShard {
			return &getTxResponse.Data.Transaction, nil
		}

		if observerIsInDestShard {
			// need to get transaction from source shard and merge scResults
			// if withEvents is true
			return tp.alterTxWithScResultsFromSourceIfNeeded(txHash, &getTxResponse.Data.Transaction, withResults), nil
		}

		// get transaction from observer that is in destination shard
		txFromDstShard, ok := tp.getTxFromDestShard(txHash, rcvShardID, withResults)
		if ok {
			alteredTxFromDest := tp.mergeScResultsFromSourceAndDestIfNeeded(&getTxResponse.Data.Transaction, txFromDstShard, withResults)
			return alteredTxFromDest, nil
		}

		// return transaction from observer from source shard
		// if did not get ok responses from observers from destination shard
		return &getTxResponse.Data.Transaction, nil
	}

	return nil, errors.ErrTransactionNotFound
}

func (tp *TransactionProcessor) alterTxWithScResultsFromSourceIfNeeded(txHash string, tx *transaction.ApiTransactionResult, withResults bool) *transaction.ApiTransactionResult {
	if !withResults || len(tx.SmartContractResults) == 0 {
		return tx
	}

	observers, err := tp.getNodesInShard(tx.SourceShard, requestTypeFullHistoryNodes)
	if err != nil {
		return tx
	}

	for _, observer := range observers {
		getTxResponse, ok, _ := tp.getTxFromObserver(observer, txHash, withResults)
		if !ok {
			continue
		}

		alteredTxFromDest := tp.mergeScResultsFromSourceAndDestIfNeeded(&getTxResponse.Data.Transaction, tx, withResults)
		return alteredTxFromDest
	}

	return tx
}

func (tp *TransactionProcessor) getTxWithSenderAddr(txHash, sender string, withEvents bool) (*transaction.ApiTransactionResult, error) {
	observers, sndShardID, err := tp.getShardObserversForSender(sender, requestTypeFullHistoryNodes)
	if err != nil {
		return nil, err
	}

	for _, observer := range observers {
		getTxResponse, ok, _ := tp.getTxFromObserver(observer, txHash, withEvents)
		if !ok {
			continue
		}

		rcvShardID, err := tp.getShardByAddress(getTxResponse.Data.Transaction.Receiver)
		if err != nil {
			log.Warn("cannot compute shard ID from receiver address",
				"receiver address", getTxResponse.Data.Transaction.Receiver,
				"error", err.Error())
		}

		isIntraShard := rcvShardID == sndShardID
		if isIntraShard {
			return &getTxResponse.Data.Transaction, nil
		}

		txFromDstShard, ok := tp.getTxFromDestShard(txHash, rcvShardID, withEvents)
		if ok {
			alteredTxFromDest := tp.mergeScResultsFromSourceAndDestIfNeeded(&getTxResponse.Data.Transaction, txFromDstShard, withEvents)
			return alteredTxFromDest, nil
		}

		return &getTxResponse.Data.Transaction, nil
	}

	return nil, errors.ErrTransactionNotFound
}

func (tp *TransactionProcessor) mergeScResultsFromSourceAndDestIfNeeded(
	sourceTx *transaction.ApiTransactionResult,
	destTx *transaction.ApiTransactionResult,
	withEvents bool,
) *transaction.ApiTransactionResult {
	if !withEvents {
		return destTx
	}

	scResults := append(sourceTx.SmartContractResults, destTx.SmartContractResults...)
	scResultsNew := tp.getScResultsUnion(scResults)

	destTx.SmartContractResults = scResultsNew

	return destTx
}

func (tp *TransactionProcessor) getScResultsUnion(scResults []*transaction.ApiSmartContractResult) []*transaction.ApiSmartContractResult {
	scResultsHash := make(map[string]*transaction.ApiSmartContractResult, 0)
	for _, scResult := range scResults {
		scResultFromMap, found := scResultsHash[scResult.Hash]
		if !found {
			scResultsHash[scResult.Hash] = scResult
			continue
		}

		mergedLog := tp.mergeLogsHandler.MergeLogEvents(scResultFromMap.Logs, scResult.Logs)
		scResultsHash[scResult.Hash] = scResult
		scResultsHash[scResult.Hash].Logs = mergedLog
	}

	newSlice := make([]*transaction.ApiSmartContractResult, 0)
	for _, scResult := range scResultsHash {
		newSlice = append(newSlice, scResult)
	}

	return newSlice
}

func (tp *TransactionProcessor) getTxFromObserver(
	observer *data.NodeData,
	txHash string,
	withResults bool,
) (*data.GetTransactionResponse, bool, bool) {
	getTxResponse := &data.GetTransactionResponse{}
	apiPath := TransactionPath + txHash
	if withResults {
		apiPath += withResultsParam
	}

	respCode, err := tp.proc.CallGetRestEndPoint(observer.Address, apiPath, getTxResponse)
	if err != nil {
		log.Trace("cannot get transaction", "address", observer.Address, "error", err)

		if respCode == http.StatusTooManyRequests {
			log.Warn("too many requests while getting tx from observer", "address", observer.Address, "tx hash", txHash)
		}

		return nil, false, true
	}

	if respCode != http.StatusOK {
		return nil, false, false
	}

	return getTxResponse, true, false
}

func (tp *TransactionProcessor) getTxFromDestShard(txHash string, dstShardID uint32, withEvents bool) (*transaction.ApiTransactionResult, bool) {
	// cross shard transaction
	destinationShardObservers, err := tp.proc.GetObservers(dstShardID)
	if err != nil {
		return nil, false
	}

	apiPath := TransactionPath + txHash
	if withEvents {
		apiPath += withResultsParam
	}

	for _, dstObserver := range destinationShardObservers {
		getTxResponseDst := &data.GetTransactionResponse{}
		respCode, err := tp.proc.CallGetRestEndPoint(dstObserver.Address, apiPath, getTxResponseDst)
		if err != nil {
			log.Trace("cannot get transaction", "address", dstObserver.Address, "error", err)
			continue
		}

		if respCode != http.StatusOK {
			continue
		}

		return &getTxResponseDst.Data.Transaction, true
	}

	return nil, false
}

func (tp *TransactionProcessor) groupTxsByShard(txs []*data.Transaction) map[uint32][]*data.Transaction {
	txsMap := make(map[uint32][]*data.Transaction)
	for idx, tx := range txs {
		senderBytes, err := tp.pubKeyConverter.Decode(tx.Sender)
		if err != nil {
			continue
		}

		senderShardID, err := tp.proc.ComputeShardId(senderBytes)
		if err != nil {
			continue
		}

		tx.Index = idx
		txsMap[senderShardID] = append(txsMap[senderShardID], tx)
	}

	return txsMap
}

func (tp *TransactionProcessor) checkTransactionFields(tx *data.Transaction) error {
	_, err := tp.pubKeyConverter.Decode(tx.Sender)
	if err != nil {
		return &errors.ErrInvalidTxFields{
			Message: errors.ErrInvalidSenderAddress.Error(),
			Reason:  err.Error(),
		}
	}

	_, err = tp.pubKeyConverter.Decode(tx.Receiver)
	if err != nil {
		return &errors.ErrInvalidTxFields{
			Message: errors.ErrInvalidReceiverAddress.Error(),
			Reason:  err.Error(),
		}
	}

	if tx.ChainID == "" {
		return &errors.ErrInvalidTxFields{
			Message: "transaction must contain chainID",
			Reason:  "no chainID",
		}
	}

	if tx.Version == 0 {
		return &errors.ErrInvalidTxFields{
			Message: "transaction must contain version",
			Reason:  "no version",
		}
	}

	_, err = hex.DecodeString(tx.Signature)
	if err != nil {
		return &errors.ErrInvalidTxFields{
			Message: errors.ErrInvalidSignatureHex.Error(),
			Reason:  err.Error(),
		}
	}

	return nil
}

// ComputeTransactionHash will compute the hash of a given transaction
// TODO move to node
func (tp *TransactionProcessor) ComputeTransactionHash(tx *data.Transaction) (string, error) {
	valueBig, ok := big.NewInt(0).SetString(tx.Value, 10)
	if !ok {
		return "", ErrInvalidTransactionValueField
	}
	receiverAddress, err := tp.pubKeyConverter.Decode(tx.Receiver)
	if err != nil {
		return "", ErrInvalidAddress
	}

	senderAddress, err := tp.pubKeyConverter.Decode(tx.Sender)
	if err != nil {
		return "", ErrInvalidAddress
	}

	signatureBytes, err := hex.DecodeString(tx.Signature)
	if err != nil {
		return "", ErrInvalidSignatureBytes
	}

	regularTx := &transaction.Transaction{
		Nonce:     tx.Nonce,
		Value:     valueBig,
		RcvAddr:   receiverAddress,
		SndAddr:   senderAddress,
		GasPrice:  tx.GasPrice,
		GasLimit:  tx.GasLimit,
		Data:      tx.Data,
		ChainID:   []byte(tx.ChainID),
		Version:   tx.Version,
		Signature: signatureBytes,
	}

	txHash, err := core.CalculateHash(tp.marshalizer, tp.hasher, regularTx)
	if err != nil {
		return "", nil
	}

	return hex.EncodeToString(txHash), nil
}

func (tp *TransactionProcessor) getNodesInShard(shardID uint32, reqType requestType) ([]*data.NodeData, error) {
	if reqType == requestTypeFullHistoryNodes {
		fullHistoryNodes, err := tp.proc.GetFullHistoryNodes(shardID)
		if err == nil && len(fullHistoryNodes) > 0 {
			return fullHistoryNodes, nil
		}
	}

	observers, err := tp.proc.GetObservers(shardID)

	return observers, err
}

// GetTransactionsPool should return all transactions from all shards pool
func (tp *TransactionProcessor) GetTransactionsPool(fields string) (*data.TransactionsPool, error) {
	if !tp.shouldAllowEntireTxPoolFetch {
		return nil, errors.ErrOperationNotAllowed
	}

	txPool, err := tp.getTxPool(fields)
	if err != nil {
		return nil, err
	}

	return txPool, nil
}

// GetTransactionsPoolForShard should return transactions pool from one observer from shard
func (tp *TransactionProcessor) GetTransactionsPoolForShard(shardID uint32, fields string) (*data.TransactionsPool, error) {
	if !tp.shouldAllowEntireTxPoolFetch {
		return nil, errors.ErrOperationNotAllowed
	}

	txPool, err := tp.getTxPoolForShard(shardID, fields)
	if err != nil {
		return nil, err
	}

	return txPool, nil
}

// GetTransactionsPoolForSender should return transactions for sender from observer's pool
func (tp *TransactionProcessor) GetTransactionsPoolForSender(sender, fields string) (*data.TransactionsPoolForSender, error) {
	txPool, err := tp.getTxPoolForSender(sender, fields)
	if err != nil {
		return nil, err
	}

	return txPool, nil
}

// GetLastPoolNonceForSender should return last nonce for sender from observer's pool
func (tp *TransactionProcessor) GetLastPoolNonceForSender(sender string) (uint64, error) {
	return tp.getLastTxPoolNonceForSender(sender)
}

// GetTransactionsPoolNonceGapsForSender should return nonce gaps for sender from observer's pool
func (tp *TransactionProcessor) GetTransactionsPoolNonceGapsForSender(sender string) (*data.TransactionsPoolNonceGaps, error) {
	return tp.getTxPoolNonceGapsForSender(sender)
}

func (tp *TransactionProcessor) getShardObserversForSender(sender string, observersType requestType) ([]*data.NodeData, uint32, error) {
	sndShardID, err := tp.getShardByAddress(sender)
	if err != nil {
		return nil, 0, errors.ErrInvalidSenderAddress
	}

	observers, err := tp.getNodesInShard(sndShardID, observersType)
	if err != nil {
		return nil, 0, err
	}

	return observers, sndShardID, nil
}

func (tp *TransactionProcessor) getTxPool(fields string) (*data.TransactionsPool, error) {
	shardIDs := tp.proc.GetShardIDs()
	txs := &data.TransactionsPool{
		RegularTransactions:  make([]data.WrappedTransaction, 0),
		SmartContractResults: make([]data.WrappedTransaction, 0),
		Rewards:              make([]data.WrappedTransaction, 0),
	}
	for _, shard := range shardIDs {
		intraShardTxs, err := tp.getTxPoolForShard(shard, fields)
		if err != nil {
			continue
		}

		txs.RegularTransactions = append(txs.RegularTransactions, intraShardTxs.RegularTransactions...)
		txs.Rewards = append(txs.Rewards, intraShardTxs.Rewards...)
		txs.SmartContractResults = append(txs.SmartContractResults, intraShardTxs.SmartContractResults...)
	}

	return txs, nil
}

func (tp *TransactionProcessor) getTxPoolForShard(shardID uint32, fields string) (*data.TransactionsPool, error) {
	observers, err := tp.getNodesInShard(shardID, requestTypeObservers)
	if err != nil {
		log.Trace("cannot get observers for shard", "shard", shardID, "error", err)
		return nil, err
	}

	for _, observer := range observers {
		txs, ok := tp.getTxPoolFromObserver(observer, fields)
		if !ok {
			continue
		}

		return txs, nil
	}

	log.Trace("cannot get tx pool for shard", "shard", shardID, "error", errors.ErrTransactionsNotFoundInPool.Error())
	return nil, errors.ErrTransactionsNotFoundInPool
}

func (tp *TransactionProcessor) getTxPoolFromObserver(
	observer *data.NodeData,
	fields string,
) (*data.TransactionsPool, bool) {
	txsPoolResponse := &data.TransactionsPoolApiResponse{}
	apiPath := TransactionsPoolPath + fieldsParam + fields

	respCode, err := tp.proc.CallGetRestEndPoint(observer.Address, apiPath, txsPoolResponse)
	if err != nil {
		log.Trace("cannot get tx pool", "address", observer.Address, "error", err)

		if respCode == http.StatusTooManyRequests {
			log.Warn("too many requests while getting tx pool", "address", observer.Address)
		}

		return nil, false
	}

	if respCode != http.StatusOK {
		return nil, false
	}

	return &txsPoolResponse.Data.Transactions, true
}

func (tp *TransactionProcessor) getTxPoolForSender(sender, fields string) (*data.TransactionsPoolForSender, error) {
	observers, _, err := tp.getShardObserversForSender(sender, requestTypeObservers)
	if err != nil {
		return nil, err
	}

	txsInPool := &data.TransactionsPoolForSender{
		Transactions: []data.WrappedTransaction{},
	}
	var ok bool
	for _, observer := range observers {
		txsInPool, ok = tp.getTxPoolForSenderFromObserver(observer, sender, fields)
		if ok {
			break
		}
	}

	return txsInPool, nil
}

func (tp *TransactionProcessor) getTxPoolForSenderFromObserver(
	observer *data.NodeData,
	sender string,
	fields string,
) (*data.TransactionsPoolForSender, bool) {
	txsPoolResponse := &data.TransactionsPoolForSenderApiResponse{}
	apiPath := TransactionsPoolPath + fieldsParam + fields + bySenderParam + sender

	respCode, err := tp.proc.CallGetRestEndPoint(observer.Address, apiPath, txsPoolResponse)
	if err != nil {
		log.Trace("cannot get tx pool for sender", "address", observer.Address, "sender", sender, "error", err)

		if respCode == http.StatusTooManyRequests {
			log.Warn("too many requests while getting tx pool for sender", "address", observer.Address, "sender", sender)
		}

		return nil, false
	}

	if respCode != http.StatusOK {
		return nil, false
	}

	return &txsPoolResponse.Data.TxPool, true
}

func (tp *TransactionProcessor) getLastTxPoolNonceForSender(sender string) (uint64, error) {
	observers, _, err := tp.getShardObserversForSender(sender, requestTypeObservers)
	if err != nil {
		return 0, err
	}

	for _, observer := range observers {
		nonce, ok := tp.getLastTxPoolNonceFromObserver(observer, sender)
		if !ok {
			continue
		}

		return nonce, nil
	}

	return 0, errors.ErrTransactionsNotFoundInPool
}

func (tp *TransactionProcessor) getLastTxPoolNonceFromObserver(
	observer *data.NodeData,
	sender string,
) (uint64, bool) {
	lastNonceResponse := &data.TransactionsPoolLastNonceForSenderApiResponse{}
	apiPath := TransactionsPoolPath + lastNonceParam + bySenderParam + sender

	respCode, err := tp.proc.CallGetRestEndPoint(observer.Address, apiPath, lastNonceResponse)
	if err != nil {
		log.Trace("cannot get last nonce from tx pool", "address", observer.Address, "sender", sender, "error", err)

		if respCode == http.StatusTooManyRequests {
			log.Warn("too many requests while getting last nonce from tx pool", "address", observer.Address, "sender", sender)
		}

		return 0, false
	}

	if respCode != http.StatusOK {
		return 0, false
	}

	return lastNonceResponse.Data.Nonce, true
}

func (tp *TransactionProcessor) getTxPoolNonceGapsForSender(sender string) (*data.TransactionsPoolNonceGaps, error) {
	observers, _, err := tp.getShardObserversForSender(sender, requestTypeObservers)
	if err != nil {
		return nil, err
	}

	nonceGaps := &data.TransactionsPoolNonceGaps{
		Gaps: []data.NonceGap{},
	}
	var ok bool
	for _, observer := range observers {
		nonceGaps, ok = tp.getTxPoolNonceGapsFromObserver(observer, sender)
		if ok {
			break
		}
	}

	return nonceGaps, nil
}

func (tp *TransactionProcessor) getTxPoolNonceGapsFromObserver(
	observer *data.NodeData,
	sender string,
) (*data.TransactionsPoolNonceGaps, bool) {
	nonceGapsResponse := &data.TransactionsPoolNonceGapsForSenderApiResponse{}
	apiPath := TransactionsPoolPath + nonceGapsParam + bySenderParam + sender

	respCode, err := tp.proc.CallGetRestEndPoint(observer.Address, apiPath, nonceGapsResponse)
	if err != nil {
		log.Warn("cannot get nonce gaps from tx pool", "address", observer.Address, "sender", sender, "error", err)

		if respCode == http.StatusTooManyRequests {
			log.Warn("too many requests while getting nonce gaps from tx pool", "address", observer.Address, "sender", sender)
		}

		return nil, false
	}

	if respCode != http.StatusOK {
		return nil, false
	}

	return &nonceGapsResponse.Data.NonceGaps, true
}
