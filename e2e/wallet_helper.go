/*
 * Copyright (C) 2017 The "MysteriumNetwork/node" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package e2e

import (
	"context"
	"errors"
	"github.com/MysteriumNetwork/payments/cli/helpers"
	mysttoken "github.com/MysteriumNetwork/payments/mysttoken/generated"
	registry "github.com/MysteriumNetwork/payments/registry/generated"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/mysterium/node/tequilapi/client"
	"math/big"
	"os"
	"time"
)

//addresses should match those deployed in e2e test environment
var tokenAddress = common.HexToAddress("0x0222eb28e1651E2A8bAF691179eCfB072457f00c")
var paymentsAddress = common.HexToAddress("0x1955141ba8e77a5B56efBa8522034352c94f77Ea")

//owner of contracts and main acc with ethereum
var mainEtherAcc = common.HexToAddress("0xa754f0d31411d88e46aed455fa79b9fced122497")
var mainEtherAccPass = "localaccount"

// CliWallet represents operations which can be done with user controlled account
type CliWallet struct {
	txOpts           *bind.TransactOpts
	Owner            common.Address
	backend          *ethclient.Client
	identityRegistry registry.IdentityRegistryTransactorSession
	tokens           mysttoken.MystTokenTransactorSession
}

// RegisterIdentity registers identity with given data on behalf of user
func (wallet *CliWallet) RegisterIdentity(dto client.RegistrationStatusDTO) error {
	var Pub1 [32]byte
	var Pub2 [32]byte
	var S [32]byte
	var R [32]byte

	copy(Pub1[:], common.FromHex(dto.PublicKey.Part1))
	copy(Pub2[:], common.FromHex(dto.PublicKey.Part2))
	copy(R[:], common.FromHex(dto.Signature.R))
	copy(S[:], common.FromHex(dto.Signature.S))

	tx, err := wallet.identityRegistry.RegisterIdentity(Pub1, Pub2, dto.Signature.V, R, S)
	if err != nil {
		return err
	}
	return wallet.checkTxResult(tx)
}

// GiveEther transfers ether to given address
func (wallet *CliWallet) GiveEther(address common.Address, amount, units int64) error {

	amountInWei := new(big.Int).Mul(big.NewInt(amount), big.NewInt(units))

	nonce, err := wallet.backend.PendingNonceAt(context.Background(), wallet.Owner)
	if err != nil {
		return err
	}
	gasPrice, err := wallet.backend.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}

	tx := types.NewTransaction(nonce, address, amountInWei, params.TxGas, gasPrice, nil)

	signedTx, err := wallet.txOpts.Signer(types.HomesteadSigner{}, wallet.Owner, tx)
	if err != nil {
		return err
	}

	err = wallet.backend.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return err
	}
	return wallet.checkTxResult(signedTx)
}

// GiveTokens gives myst tokens to specified address
func (wallet *CliWallet) GiveTokens(address common.Address, amount int64) error {
	tx, err := wallet.tokens.Mint(address, big.NewInt(amount))
	if err != nil {
		return err
	}
	return wallet.checkTxResult(tx)
}

// ApproveForPayments allows specified amount of ERC20 tokens to be spend by payments contract
func (wallet *CliWallet) ApproveForPayments(amount int64) error {
	tx, err := wallet.tokens.Approve(paymentsAddress, big.NewInt(amount))
	if err != nil {
		return err
	}
	return wallet.checkTxResult(tx)
}

func (wallet *CliWallet) checkTxResult(tx *types.Transaction) error {
	for i := 0; i < 10; i++ {
		_, pending, err := wallet.backend.TransactionByHash(context.Background(), tx.Hash())
		switch {
		case err != nil:
			return err
		case pending:
			time.Sleep(1 * time.Second)
		case !pending:
			break
		}
	}

	receipt, err := wallet.backend.TransactionReceipt(context.Background(), tx.Hash())
	if err != nil {
		return err
	}
	if receipt.Status != 1 {
		return errors.New("tx marked as failed")
	}
	return nil
}

// NewMainAccWallet initializes wallet with main localnet account private key (owner of ERC20, payments and lots of ether)
func NewMainAccWallet(keystoreDir string) (*CliWallet, error) {
	ks := initKeyStore(keystoreDir)

	return newCliWallet(mainEtherAcc, mainEtherAccPass, ks)
}

// NewUserWallet initializes wallet with generated account with specified keystore
func NewUserWallet(keystoreDir string) (*CliWallet, error) {
	ks := initKeyStore(keystoreDir)
	acc, err := ks.NewAccount("")
	if err != nil {
		return nil, err
	}
	return newCliWallet(acc.Address, "", ks)
}

func newCliWallet(owner common.Address, passphrase string, ks *keystore.KeyStore) (*CliWallet, error) {
	client, err := newEthClient()
	if err != nil {
		return nil, err
	}

	ownerAcc := accounts.Account{Address: owner}

	err = ks.Unlock(ownerAcc, passphrase)
	if err != nil {
		return nil, err
	}

	transactor := helpers.CreateNewKeystoreTransactor(ks, &ownerAcc)

	tokensContract, err := mysttoken.NewMystTokenTransactor(tokenAddress, client)

	paymentsContract, err := registry.NewIdentityRegistryTransactor(paymentsAddress, client)
	if err != nil {
		return nil, err
	}

	return &CliWallet{
		txOpts:  transactor,
		Owner:   owner,
		backend: client,
		tokens: mysttoken.MystTokenTransactorSession{
			Contract:     tokensContract,
			TransactOpts: *transactor,
		},
		identityRegistry: registry.IdentityRegistryTransactorSession{
			Contract:     paymentsContract,
			TransactOpts: *transactor,
		},
	}, nil
}

func initKeyStore(path string) *keystore.KeyStore {
	return keystore.NewKeyStore(path, keystore.StandardScryptN, keystore.StandardScryptP)
}

func registerIdentity(registrationData client.RegistrationStatusDTO) error {
	defer os.RemoveAll("testdataoutput")

	//master account - owner of conctracts, and can issue tokens
	masterAccWallet, err := NewMainAccWallet("../bin/localnet/account")
	if err != nil {
		return err
	}

	//random user
	userWallet, err := NewUserWallet("testdataoutput")
	if err != nil {
		return err
	}

	//user gets some ethers from master acc
	err = masterAccWallet.GiveEther(userWallet.Owner, 1, params.Ether)
	if err != nil {
		return err
	}

	//user buys some tokens in exchange
	err = masterAccWallet.GiveTokens(userWallet.Owner, 1000)
	if err != nil {
		return err
	}

	//user allows payments to take some tokens
	err = userWallet.ApproveForPayments(1000)
	if err != nil {
		return err
	}

	//user registers identity
	err = userWallet.RegisterIdentity(registrationData)
	return err
}
