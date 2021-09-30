package organizations

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"zuri.chat/zccore/utils"
)

const (
	USD = "usd" // US Dollar ($)
	EUR = "eur" // Euro (€)
	GBP = "gbp" // British Pound Sterling (UK£)
	JPY = "jpy" // Japanese Yen (¥)
	CAD = "cad" // Canadian Dollar (CA$)
	HKD = "hkd" // Hong Kong Dollar (HK$)
	CNY = "cny" // Chinese Yuan (CN¥)
	AUD = "aud" // Australian Dollar (A$)
)

// converts amount in naira to equivalent token value
func GetTokenAmount(Amount float64, Currency string) (float64, error) {
	var ExchangeMap = map[string]float64{
		USD: 1,
		EUR: 0.86,
	}
	ConversionRate, ok := ExchangeMap[Currency]
	if !ok {
		return float64(0), errors.New("currency not yet supported")
	}
	return Amount * ConversionRate, nil
}

// takes as input org id and token amount and increments token by that amount
func IncrementToken(OrgId string, TokenAmount float64) error {
	OrgIdFromHex, err := primitive.ObjectIDFromHex(OrgId)
	if err != nil {
		return err
	}

	organization, err := FetchOrganization(bson.M{"_id": OrgIdFromHex})
	if err != nil {
		return err
	}

	organization.Tokens += TokenAmount
	update_data := make(map[string]interface{})
	update_data["tokens"] = organization.Tokens
	if _, err := utils.UpdateOneMongoDbDoc(OrganizationCollectionName, OrgId, update_data); err != nil {
		return err
	}
	return nil
}

// takes as input org id and token amount and decreases token by that amount if available, else returns error
func DeductToken(OrgId string, TokenAmount float64) error {

	OrgIdFromHex, err := primitive.ObjectIDFromHex(OrgId)
	if err != nil {
		return err
	}

	organization, err := FetchOrganization(bson.M{"_id": OrgIdFromHex})
	if err != nil {
		return err
	}

	if organization.Tokens < TokenAmount {
		return errors.New("insufficient token balance")
	}

	organization.Tokens -= TokenAmount
	update_data := make(map[string]interface{})
	update_data["tokens"] = organization.Tokens
	if _, err := utils.UpdateOneMongoDbDoc(OrganizationCollectionName, OrgId, update_data); err != nil {
		return err
	}
	return nil
}

func SubscriptionBilling(OrgId string, ProVersionRate float64) error {

	orgMembers, err := utils.GetMongoDbDocs(MemberCollectionName, bson.M{"org_id": OrgId})
	if err != nil {
		return err
	}

	amount := float64(len(orgMembers)) * ProVersionRate

	if err := DeductToken(OrgId, amount); err != nil {
		return err
	}
	return nil
}

func SendTokenBillingEmail() {

}

// allows user to be able to load tokens into organization wallet
func (oh *OrganizationHandler) AddToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	orgId := mux.Vars(r)["id"]
	objId, err := primitive.ObjectIDFromHex(orgId)

	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	org, _ := utils.GetMongoDbDoc(OrganizationCollectionName, bson.M{"_id": objId})

	if org == nil {
		utils.GetError(fmt.Errorf("organization %s not found", orgId), http.StatusNotFound, w)
		return
	}

	requestData := make(map[string]float64)
	if err := utils.ParseJsonFromRequest(r, &requestData); err != nil {
		utils.GetError(err, http.StatusUnprocessableEntity, w)
		return
	}

	org_filter := make(map[string]interface{})
	tokens, ok := requestData["amount"]
	if !ok {
		utils.GetError(errors.New("amount not supplied"), http.StatusUnprocessableEntity, w)
		return
	}

	org_filter["tokens"] = org["tokens"].(float64) + (tokens * 0.2)

	update, err := utils.UpdateOneMongoDbDoc(OrganizationCollectionName, orgId, org_filter)
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}
	var transaction TokenTransaction

	transaction.Amount = tokens
	transaction.Currency = "usd"
	transaction.Description = "Purchase Token"
	transaction.OrgId = orgId
	transaction.TransactionId = utils.GenUUID()
	transaction.Type = "Purchase"
	transaction.Time = time.Now()
	transaction.Token = tokens * 0.2
	detail, _ := utils.StructToMap(transaction)

	res, err := utils.CreateMongoDbDoc(TokenTransactionCollectionName, detail)

	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}
	if update.ModifiedCount == 0 {
		utils.GetError(errors.New("operation failed"), http.StatusInternalServerError, w)
		return
	}

	utils.GetSuccess("Successfully loaded token", res, w)

}

// Get an organization transaction record
func (oh *OrganizationHandler) GetTokenTransaction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	orgId := mux.Vars(r)["id"]

	save, _ := utils.GetMongoDbDocs(TokenTransactionCollectionName, bson.M{"org_id": orgId})

	if save == nil {
		utils.GetError(fmt.Errorf("organization transaction %s not found", orgId), http.StatusNotFound, w)
		return
	}

	utils.GetSuccess("transactions retrieved successfully", save, w)
}
