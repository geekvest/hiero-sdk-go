package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hiero-ledger/hiero-sdk-go/v2/examples/contract_helper"
	hiero "github.com/hiero-ledger/hiero-sdk-go/v2/sdk"
)

type Contract struct {
	Bytecode string `json:"bytecode"`
}

// Steps 1-5 are executed through ContractHelper and calling HIP564Example Contract.
// Step 6 is executed through the SDK
func main() {
	var client *hiero.Client
	var err error
	var contract Contract
	// Retrieving network type from environment variable HEDERA_NETWORK, i.e. testnet
	client, err = hiero.ClientForName(os.Getenv("HEDERA_NETWORK"))
	if err != nil {
		panic(fmt.Sprintf("%v : error creating client", err))
	}

	// Grab your testnet account ID and private key from the environment variable
	myAccountId, err := hiero.AccountIDFromString(os.Getenv("OPERATOR_ID"))
	if err != nil {
		panic(err)
	}

	myPrivateKey, err := hiero.PrivateKeyFromStringEd25519(os.Getenv("OPERATOR_KEY"))
	if err != nil {
		panic(err)
	}

	// Print your testnet account ID and private key to the console to make sure there was no error
	fmt.Printf("The account ID is = %v\n", myAccountId)
	fmt.Printf("The private key is = %v\n", myPrivateKey)

	client.SetOperator(myAccountId, myPrivateKey)
	// Generate new keys for the account you will create
	alicePrivateKey, err := hiero.PrivateKeyGenerateEd25519()
	if err != nil {
		panic(err)
	}

	newAccountPublicKey := alicePrivateKey.PublicKey()

	// Create new account and assign the public key
	aliceAccount, err := hiero.NewAccountCreateTransaction().
		SetKeyWithoutAlias(newAccountPublicKey).
		SetInitialBalance(hiero.HbarFrom(1000, hiero.HbarUnits.Tinybar)).
		Execute(client)
	if err != nil {
		panic(err)
	}
	// Request the receipt of the transaction
	receipt, err := aliceAccount.GetReceipt(client)
	if err != nil {
		panic(err)
	}

	// Get the new account ID from the receipt
	aliceAccountId := *receipt.AccountID
	fmt.Println("aliceAcountid is: ", aliceAccountId)

	// Transfer hbar from your testnet account to the new account
	transaction := hiero.NewTransferTransaction().
		AddHbarTransfer(myAccountId, hiero.HbarFrom(-1000, hiero.HbarUnits.Tinybar)).
		AddHbarTransfer(aliceAccountId, hiero.HbarFrom(1000, hiero.HbarUnits.Tinybar))

	// Submit the transaction to a Hiero network
	_, err = transaction.Execute(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error submitting transaction", err))
	}

	rawContract, err := os.ReadFile("../precompile_example/ZeroTokenOperations.json")
	if err != nil {
		panic(fmt.Sprintf("%v : error reading json", err))
	}

	err = json.Unmarshal(rawContract, &contract)
	if err != nil {
		panic(fmt.Sprintf("%v : error unmarshaling the json file", err))
	}
	params, err := hiero.NewContractFunctionParameters().AddAddress(myAccountId.ToEvmAddress())
	if err != nil {
		panic(fmt.Sprintf("%v : error adding first address to contract function parameters", err))
	}
	params, err = params.AddAddress(aliceAccountId.ToEvmAddress())
	if err != nil {
		panic(fmt.Sprintf("%v : error adding second address to contract function parameters", err))
	}

	helper := contract_helper.NewContractHelper([]byte(contract.Bytecode), *params, client)
	helper.SetPayableAmountForStep(0, hiero.NewHbar(20)).AddSignerForStep(1, alicePrivateKey)

	keyList := hiero.KeyListWithThreshold(1).Add(myPrivateKey.PublicKey()).Add(helper.ContractID)
	frozenTxn, err := hiero.NewAccountUpdateTransaction().SetAccountID(myAccountId).SetKey(keyList).FreezeWith(client)
	if err != nil {
		panic(err)
	}
	tx, err := frozenTxn.Sign(myPrivateKey).Execute(client)
	if err != nil {
		panic(err)
	}
	_, err = tx.GetReceipt(client)
	if err != nil {
		panic(err)
	}
	keyList = hiero.KeyListWithThreshold(1).Add(alicePrivateKey.PublicKey()).Add(helper.ContractID)

	frozenTxn, err = hiero.NewAccountUpdateTransaction().SetAccountID(aliceAccountId).SetKey(keyList).FreezeWith(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error updating alice's account", err))
	}
	tx, err = frozenTxn.Sign(alicePrivateKey).Execute(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error updating alice's account", err))
	}
	_, err = tx.GetReceipt(client)
	if err != nil {
		panic(err)
	}

	// TODO there is currently possible bug in services causing this operation to fail, should be investigated
	// _, err = helper.ExecuteSteps(0, 5, client)
	// if err != nil {
	// 	panic(fmt.Sprintf("%v : error in helper", err))
	// }
	transactionResponse, err := hiero.NewTokenCreateTransaction().
		SetTokenName("Black Sea LimeChain Token").
		SetTokenSymbol("BSL").
		SetTreasuryAccountID(myAccountId).
		SetInitialSupply(10000).
		SetDecimals(2).
		SetAutoRenewAccount(myAccountId).
		SetFreezeDefault(false).
		Execute(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error creating token", err))
	}

	// Make sure the token create transaction ran
	transactionReceipt, err := transactionResponse.GetReceipt(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error retrieving token creation receipt", err))
	}

	// Retrieve the token out of the receipt
	tokenID := *transactionReceipt.TokenID

	fmt.Printf("token = %v\n", tokenID.String())

	// Associating the token with the second account, so it can interact with the token
	associatingTransaction, err := hiero.NewTokenAssociateTransaction().
		// The account ID to be associated
		SetAccountID(aliceAccountId).
		SetNodeAccountIDs([]hiero.AccountID{transactionResponse.NodeID}).
		// The token ID that the account will be associated to
		SetTokenIDs(tokenID).
		FreezeWith(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error freezing second token associate transaction", err))
	}
	// Has to be signed by the account2's key
	transactionResponse, err = associatingTransaction.
		Sign(alicePrivateKey).
		Execute(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error executing second token associate transaction", err))
	}

	// Make sure the transaction succeeded
	transactionReceipt, err = transactionResponse.GetReceipt(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error retrieving second token associate transaction receipt", err))
	}

	fmt.Printf("Associated account %v with token %v\n", aliceAccountId.String(), tokenID.String())

	// Transfer 0 tokens
	transactionResponse, err = hiero.NewTransferTransaction().
		AddTokenTransfer(tokenID, myAccountId, 0).AddTokenTransfer(tokenID, aliceAccountId, 0).Execute(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error transferring token", err))
	}
	_, err = transactionResponse.GetRecord(client)
	if err != nil {
		panic(fmt.Sprintf("%v : error retrieving transaction", err))
	}
}
