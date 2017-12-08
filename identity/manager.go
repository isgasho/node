// Maps Ethereum account to dto.Identity.
// Currently creates a new eth account with password on CreateNewIdentity().

package identity

import (
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mysterium/node/service_discovery/dto"
	"strings"
)

type IdentityManager struct {
	keystoreManager keystoreInterface
}

func NewIdentityManager(keystore keystoreInterface) *IdentityManager {
	return &IdentityManager{
		keystoreManager: keystore,
	}
}

func accountToIdentity(account accounts.Account) *dto.Identity {
	identity := dto.Identity(account.Address.Hex())
	return &identity
}

func identityToAccount(identityString string) accounts.Account {
	return accounts.Account{
		Address: common.HexToAddress(identityString),
	}
}

func (idm *IdentityManager) CreateNewIdentity(passphrase string) (*dto.Identity, error) {
	account, err := idm.keystoreManager.NewAccount(passphrase)
	if err != nil {
		return nil, err
	}

	return accountToIdentity(account), nil
}

func (idm *IdentityManager) GetIdentities() []dto.Identity {
	accountList := idm.keystoreManager.Accounts()

	var ids = make([]dto.Identity, len(accountList))
	for i, account := range accountList {
		ids[i] = *accountToIdentity(account)
	}

	return ids
}

func (idm *IdentityManager) GetIdentity(identityString string) *dto.Identity {
	identityString = strings.ToLower(identityString)
	for _, id := range idm.GetIdentities() {
		if strings.ToLower(string(id)) == identityString {
			return &id
		}
	}

	return nil
}

func (idm *IdentityManager) HasIdentity(identityString string) bool {
	return idm.GetIdentity(identityString) != nil
}
