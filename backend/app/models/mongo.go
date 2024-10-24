package models

type MongoUser struct {
	Email            string            `bson:"email"`
	ID               string            `bson:"_id"`
	Name             string            `bson:"name"`
	Phone            string            `bson:"phone"`
	StripeCustomerId string            `bson:"stripe_customer_id"`
	SubscriptionDate string            `bson:"subscription_date"`
	SubscriptionType MongoSubscription `bson:"subscription"`
	Usage            float64           `bson:"usage"`
	LastUsedAt       string            `bson:"last_used_at"`
	LastNotifiedAt   string            `bson:"last_notified_at"`

	// Start params
	Source   string `bson:"source"`
	Mode     string `bson:"mode"`
	Language string `bson:"language"`
}

type MongoSubscription struct {
	Name         MongoSubscriptionName `bson:"name"`
	MaximumUsage float64               `bson:"maximum_usage"`
}

type MongoInvoice struct {
	ID                      string `bson:"_id"`
	UserID                  string `bson:"user_id"`
	Amount                  int    `bson:"amount"`
	Currency                string `bson:"currency"`
	CreatedAt               string `bson:"created_at"`
	TelegramPaymentChargeID string `bson:"telegram_payment_charge_id"`
	ProviderPaymentChargeID string `bson:"provider_payment_charge_id"`
}

type MongoSubscriptionName string

type MongoUserThread struct {
	ID         string `bson:"_id"`
	CreatedAt  string `bson:"created_at"`
	ThreadJson string `bson:"thread_json"`
	UpdateAt   string `bson:"updated_at"`
	UserId     string `bson:"user_id"`
}
