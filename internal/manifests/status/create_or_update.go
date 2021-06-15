package status

// OperationResultType resprents the enumeration type for all
// possible result values of create or update operations.
type OperationResultType string

const (
	// OperationResultNone when neither a create nor a update
	// operation needed.
	OperationResultNone OperationResultType = "unchanged"
	// OperationResultCreated when only a create operation
	// executed successfully.
	OperationResultCreated OperationResultType = "created"
	// OperationResultUpdated when only an update operation
	// executed successfully.
	OperationResultUpdated OperationResultType = "updated"
)
