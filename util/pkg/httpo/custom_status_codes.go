package httpo

const (

	// Half success

	// Occurs when transaction is successful but failed to update in DB
	TXDbFailed = 5001
	
	// Access issues

	// Occurs when token is expired
	TokenExpired = 4031

	// Occurs when token is invalid, for example, signed by wrong signature or is malformed
	TokenInvalid = 4033

	// Occurs when signatures public key doesn't match to the one which was used while requesting challenge
	SignatureDenied = 4034

	// Occues when user is locked and cannot perform certain operations
	UserLocked = 4035

	// Request issues

	// The header doesn't contain Authorization header or it is empty
	AuthHeaderMissing = 4001

	// The provided string is not valid base64
	InvalidBase64 = 4002

	// The provided wallet address is not compatible to the chain
	WalletAddressInvalid = 4003

	// The total cart amount doesn't match amount calculated on server
	CartTotalIncorrect = 4004
	
	// Item already exist
	ItemAlreadyExist = 4005
	
	// Item doesn't exist (also foreign key violation)
	ItemDoesNotExist = 4006

	// State issues

	// User trying to refer doesn't exist in database
	UserNotFound = 4041

	// FlowID trying to refer doesn't exist in database
	FlowIdNotFound = 4042

	// Account trying to refer in chain doesn't exist, this means that account doesn't have any in or out transactions and therefore it has 0 balance
	AccountNotFound = 4043

	// Service trying to refer by id doesn't exist
	ServiceNotFound = 4044
)
