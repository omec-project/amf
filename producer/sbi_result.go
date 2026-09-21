// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package producer

// anyOrNil boxes a pointer for the interface-typed fields of context.SbiResponseMsg, and
// returns an untyped nil when the pointer is nil.
//
// The SBI handlers hand their results back as interface{}, and a nil *T stored in an
// interface is not nil -- the interface carries the type. Every caller tests the field it
// was given, so `if msg.ProblemDetails != nil` is true on a successful procedure, and the
// three ways that goes wrong are all reachable today:
//
//   - a caller that dereferences inline answers with the status of a nil *ProblemDetails,
//     which is 0, so a successful UE context release returns HTTP 0 where the code reads 204;
//   - a caller that maps status 0 to 500 answers 500 for a successful ProvideLocationInfo or
//     ProvideDomainSelectionInfo;
//   - a caller that type-asserts RespData gets ok == true and a nil pointer, so a *failed*
//     registration status update answers 200 with a null body instead of its problem details.
//
// Boxing here rather than testing at each of the twenty-five call sites keeps the invariant
// where the value is produced: what leaves a handler is nil exactly when the procedure
// returned nil.
func anyOrNil[T any](p *T) any {
	if p == nil {
		return nil
	}

	return p
}
