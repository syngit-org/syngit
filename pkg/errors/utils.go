package errors

type SyngitError interface {
	ShouldContains(err error) bool
}

func Is(err error, target SyngitError) bool {
	return target.ShouldContains(err)
}
