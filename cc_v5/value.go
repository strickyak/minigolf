// Copyright 2022 The CC Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cc // import "modernc.org/cc/v5"

import (
	"math/big"
)

var (
	_ Value = (*ComplexLongDoubleValue)(nil)
	_ Value = (*LongDoubleValue)(nil)
	_ Value = (*UnknownValue)(nil)
	_ Value = (*ZeroValue)(nil)
	_ Value = Complex128Value(0)
	_ Value = Complex64Value(0)
	_ Value = Float64Value(0)
	_ Value = Int64Value(0)
	_ Value = StringValue("")
	_ Value = UInt64Value(0)
	_ Value = UTF16StringValue(nil)
	_ Value = UTF32StringValue(nil)
	_ Value = VoidValue{}
)

var (
	// Unknown is a singleton representing an undetermined value.  Unknown is
	// comparable.
	Unknown Value = &UnknownValue{}

	// Zero is a singleton representing a zero value of a type. Zero is comparable.
	Zero Value = &ZeroValue{}

	int1 = Int64Value(1)
	int0 = Int64Value(0)
)

type valuer struct{ val Value }

// Value returns the value of a node or UnknownValue if it is undetermined. The
// dynamic type of a Value is one of
//
//	*ComplexLongDoubleValue
//	*LongDoubleValue
//	*UnknownValue
//	*ZeroValue
//	Complex128Value
//	Complex64Value
//	Float64Value
//	Int64Value
//	StringValue
//	UInt64Value
//	UTF16StringValue
//	UTF32StringValue
//	VoidValue
func (v valuer) Value() Value {
	if v.val != nil {
		return v.val
	}

	return Unknown
}

type Value interface {
	// Convert attempts to convert the value to a different type. It returns
	// Unknown values unchanged.  Convert returns Unknown for values it does not
	// support.
	Convert(to Type) Value
}

type UnknownValue struct{}

func (*UnknownValue) Convert(to Type) Value { return Unknown }

func (*UnknownValue) String() string { return "<unknown value>" }

type ZeroValue struct{}

func (*ZeroValue) isValue() {}

func (n *ZeroValue) Convert(to Type) Value { return convert(n, to) }

type ComplexLongDoubleValue struct {
	Re *LongDoubleValue
	Im *LongDoubleValue
}

func (n *ComplexLongDoubleValue) Convert(to Type) Value { return convert(n, to) }

type LongDoubleValue big.Float

func (n *LongDoubleValue) Convert(to Type) Value { return convert(n, to) }

type Complex128Value complex128

func (n Complex128Value) Convert(to Type) Value { return convert(n, to) }

type Complex64Value complex64

func (n Complex64Value) Convert(to Type) Value { return convert(n, to) }

type Float64Value float64

func (n Float64Value) Convert(to Type) Value { return convert(n, to) }

type Int64Value int64

func (n Int64Value) Convert(to Type) Value { return convert(n, to) }

type UInt64Value uint64

func (n UInt64Value) Convert(to Type) Value { return convert(n, to) }

type VoidValue struct{}

func (n VoidValue) Convert(to Type) Value { return convert(n, to) }

type StringValue string

func (n StringValue) Convert(to Type) Value { return convert(n, to) }

type UTF16StringValue []uint16

func (n UTF16StringValue) Convert(to Type) Value { return convert(n, to) }

type UTF32StringValue []rune

func (n UTF32StringValue) Convert(to Type) Value { return convert(n, to) }

func (n *ConstantExpression) eval(c *ctx, mode flags) (r Value) {
	n.val = n.Expression.eval(c, mode)
	return n.Value()
}

func (n *ConditionalExpression) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %v", mode.has(addrOf)))
		return n.Value()
	}

	switch val := n.Condition.eval(c, mode); {
	case isNonzero(val):
		n.val = convert(n.Then.eval(c, mode), n.Type())
	case isZero(val):
		n.val = convert(n.Else.eval(c, mode), n.Type())
	}
	return n.Value()
}

func (n *BinaryExpression) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %v %v", n.Op, mode.has(addrOf)))
		return n.Value()
	}

	switch n.Op {
	case BinaryOperationAdd:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				n.val = convert(x+y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case UInt64Value:
				n.val = convert(x+y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case Float64Value:
			// nop
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationSub:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case UInt64Value:
				n.val = convert(x-y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				n.val = convert(x-y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationMul:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case UInt64Value:
				n.val = convert(x*y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				n.val = convert(x*y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationDiv:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				if y != 0 {
					n.val = convert(x/y, n.Type())
					break
				}
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case UInt64Value:
				if y != 0 {
					n.val = convert(x/y, n.Type())
					break
				}
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationMod:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				if y != 0 {
					n.val = convert(x%y, n.Type())
					break
				}
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case UInt64Value:
				if y != 0 {
					n.val = convert(x%y, n.Type())
					break
				}
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationLOr:
		switch v := convert(n.Lhs.eval(c, mode), n.Type()); {
		case isZero(v):
			switch v := convert(n.Rhs.eval(c, mode), n.Type()); {
			case isZero(v):
				n.val = int0
			case isNonzero(v):
				n.val = int1
			}
		case isNonzero(v):
			n.val = int1
		}
	case BinaryOperationLAnd:
		switch v := n.Lhs.eval(c, mode); {
		case isZero(v):
			n.val = int0
		case isNonzero(v):
			switch w := n.Rhs.eval(c, mode); {
			case isZero(w):
				n.val = int0
			case isNonzero(w):
				n.val = int1
			}
		}
	case BinaryOperationOr:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				n.val = convert(x|y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// nop
			case UInt64Value:
				n.val = convert(x|y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationXor:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// ok
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				n.val = convert(x^y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// ok
			case UInt64Value:
				n.val = convert(x^y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationAnd:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// ok
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				n.val = convert(x&y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), n.Type()).(type) {
			case *UnknownValue:
				// ok
			case UInt64Value:
				n.val = convert(x&y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationEq:
		t1 := n.Lhs.Type()
		t2 := n.Rhs.Type()
		if IsArithmeticType(t1) && IsArithmeticType(t2) {
			t1 = UsualArithmeticConversions(t1, t2)
			t2 = t1
		}
		switch x := convert(n.Lhs.eval(c, mode), t1).(type) {
		case *UnknownValue:
			// ok
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				n.val = bool2int(x == y)
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case UInt64Value:
				n.val = bool2int(x == y)
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationNeq:
		t1 := n.Lhs.Type()
		t2 := n.Rhs.Type()
		if IsArithmeticType(t1) && IsArithmeticType(t2) {
			t1 = UsualArithmeticConversions(t1, t2)
			t2 = t1
		}
		switch x := convert(n.Lhs.eval(c, mode), t1).(type) {
		case *UnknownValue:
			// ok
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				n.val = bool2int(x != y)
			case UInt64Value:
				n.val = bool2int(UInt64Value(x) != y)
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				n.val = bool2int(x != UInt64Value(y))
			case UInt64Value:
				n.val = bool2int(x != y)
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationLt:
		t1 := n.Lhs.Type()
		t2 := n.Rhs.Type()
		if IsArithmeticType(t1) && IsArithmeticType(t2) {
			t1 = UsualArithmeticConversions(t1, t2)
			t2 = t1
		}
		switch x := convert(n.Lhs.eval(c, mode), t1).(type) {
		case *UnknownValue:
			// ok
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), t1).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				if x < y {
					n.val = int1
					break
				}

				n.val = int0
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case UInt64Value:
				if x < y {
					n.val = int1
					break
				}

				n.val = int0
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationGt:
		t1 := n.Lhs.Type()
		t2 := n.Rhs.Type()
		if IsArithmeticType(t1) && IsArithmeticType(t2) {
			t1 = UsualArithmeticConversions(t1, t2)
			t2 = t1
		}
		switch x := convert(n.Lhs.eval(c, mode), t1).(type) {
		case *UnknownValue:
			// ok
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				if x > y {
					n.val = int1
					break
				}

				n.val = int0
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case UInt64Value:
				if x > y {
					n.val = int1
					break
				}

				n.val = int0
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationLeq:
		t1 := n.Lhs.Type()
		t2 := n.Rhs.Type()
		if IsArithmeticType(t1) && IsArithmeticType(t2) {
			t1 = UsualArithmeticConversions(t1, t2)
			t2 = t1
		}
		switch x := convert(n.Lhs.eval(c, mode), t1).(type) {
		case *UnknownValue:
			// ok
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				if x <= y {
					n.val = int1
					break
				}

				n.val = int0
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case UInt64Value:
				if x <= y {
					n.val = int1
					break
				}

				n.val = int0
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationGeq:
		t1 := n.Lhs.Type()
		t2 := n.Rhs.Type()
		if IsArithmeticType(t1) && IsArithmeticType(t2) {
			t1 = UsualArithmeticConversions(t1, t2)
			t2 = t1
		}
		switch x := convert(n.Lhs.eval(c, mode), t1).(type) {
		case *UnknownValue:
			// ok
		case Int64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case Int64Value:
				if x >= y {
					n.val = int1
					break
				}

				n.val = int0
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := convert(n.Rhs.eval(c, mode), t2).(type) {
			case *UnknownValue:
				// ok
			case UInt64Value:
				if x >= y {
					n.val = int1
					break
				}

				n.val = int0
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationLsh:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			switch y := n.Rhs.eval(c, mode).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				n.val = convert(x<<y, n.Type())
			case UInt64Value:
				n.val = convert(x<<y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := n.Rhs.eval(c, mode).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				n.val = convert(x<<y, n.Type())
			case UInt64Value:
				n.val = convert(x<<y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	case BinaryOperationRsh:
		switch x := convert(n.Lhs.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			switch y := n.Rhs.eval(c, mode).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				n.val = convert(x>>y, n.Type())
			case UInt64Value:
				n.val = convert(x>>y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		case UInt64Value:
			switch y := n.Rhs.eval(c, mode).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				n.val = convert(x>>y, n.Type())
			case UInt64Value:
				n.val = convert(x>>y, n.Type())
			default:
				c.errors.add(errorf("TODO %v TYPE %T", n.Op, y))
			}
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Op, x))
		}
	default:
		c.errors.add(errorf("internal error: %v", n.Op))
	}
	return n.Value()
}

func (n *CastExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %v", mode.has(addrOf)))
		return n.Value()
	}

	n.val = convert(n.Expr.eval(c, mode), n.TypeName.Type())
	return n.Value()
}

func (n *PrefixExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %T", n))
		return n.Value()
	}

	// nop
	return n.Value()
}

func (n *UnaryExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		switch n.Case {
		case UnaryExpressionAddrof: // '&' CastExpression
			c.errors.add(errorf("TODO %T %v", n, n.Case))
		case UnaryExpressionDeref: // '*' CastExpression
			switch x := n.Expr.eval(c, mode.del(addrOf)).(type) {
			case *UnknownValue:
				// ok
			case UInt64Value:
				if _, ok := n.Expr.Type().(*PointerType); ok {
					n.val = convert(x, n.Expr.Type())
				}
			default:
				c.errors.add(errorf("TODO %v %v %T", n.Case, mode.has(addrOf), x))
			}
		case UnaryExpressionPlus: // '+' CastExpression
			c.errors.add(errorf("TODO %T %v", n, n.Case))
		case UnaryExpressionMinus: // '-' CastExpression
			c.errors.add(errorf("TODO %T %v", n, n.Case))
		case UnaryExpressionCpl: // '~' CastExpression
			c.errors.add(errorf("TODO %T %v", n, n.Case))
		case UnaryExpressionNot: // '!' CastExpression
			c.errors.add(errorf("TODO %T %v", n, n.Case))
		case UnaryExpressionImag: // "__imag__" UnaryExpression
			c.errors.add(errorf("TODO %T %v", n, n.Case))
		case UnaryExpressionReal: // "__real__" UnaryExpression
			switch x := n.Expr.eval(c, mode.del(addrOf)).(type) {
			case *UnknownValue:
				// ok
			default:
				c.errors.add(errorf("TODO %v %v %T", n.Case, mode.has(addrOf), x))
			}
		default:
			c.errors.add(errorf("internal error: %v", n.Case))
		}
		return n.Value()
	}

	switch n.Case {
	case UnaryExpressionAddrof: // '&' CastExpression
		n.val = convert(n.Expr.eval(c, mode.add(addrOf)), n.Type())
	case UnaryExpressionDeref: // '*' CastExpression
		// nop
	case UnaryExpressionPlus: // '+' CastExpression
		switch x := convert(n.Expr.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			n.val = convert(x, n.Type())
		case UInt64Value:
			n.val = convert(x, n.Type())
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Case, x))
		}
	case UnaryExpressionMinus: // '-' CastExpression
		switch x := convert(n.Expr.eval(c, mode), n.Type()).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			n.val = convert(-x, n.Type())
		case UInt64Value:
			n.val = convert(-x, n.Type())
		case Float64Value:
			// nop
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Case, x))
		}
	case UnaryExpressionCpl: // '~' CastExpression
		switch x := n.Expr.eval(c, mode).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			n.val = convert(^x, n.Type())
		case UInt64Value:
			n.val = convert(^x, n.Type())
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Case, x))
		}
	case UnaryExpressionNot: // '!' CastExpression
		switch x := n.Expr.eval(c, mode).(type) {
		case *UnknownValue:
			// nop
		case Int64Value:
			n.val = convert(bool2int(x == 0), n.Type())
		case UInt64Value:
			n.val = convert(bool2int(x == 0), n.Type())
		case StringValue, UTF16StringValue, UTF32StringValue:
			n.val = convert(int0, n.Type())
		default:
			c.errors.add(errorf("TODO %v TYPE %T", n.Case, x))
		}
	case UnaryExpressionImag: // "__imag__" UnaryExpression
		// nop
	case UnaryExpressionReal: // "__real__" UnaryExpression
		// nop
	default:
		c.errors.add(errorf("internal error: %v", n.Case))
	}
	return n.Value()
}

func (n *SizeOfExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %T", n))
		return n.Value()
	}

	// nop
	return n.Value()
}

func (n *SizeOfTypeExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %T", n))
		return n.Value()
	}

	// nop
	return n.Value()
}

func (n *LabelAddrExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %T", n))
		return n.Value()
	}

	// nop
	return n.Value()
}

func (n *AlignOfExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %T", n))
		return n.Value()
	}

	// nop
	return n.Value()
}

func (n *AlignOfTypeExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %T", n))
		return n.Value()
	}

	// nop
	return n.Value()
}

func (n *IndexExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		switch x := n.Expr.eval(c, mode.del(addrOf)).(type) {
		case *UnknownValue, StringValue, UTF16StringValue, UTF32StringValue:
			// ok
		case UInt64Value:
			if p, ok := n.Expr.Type().(*PointerType); ok {
				if ix, ok := int64Value(c, n.Index); ok && ix >= 0 {
					if esz := p.Elem().Size(); esz >= 0 {
						n.val = convert(x+UInt64Value(ix*esz), c.newPointerType(p.Elem()))
					}
				}
			}
		default:
			c.errors.add(errorf("TODO %v %T", mode.has(addrOf), x))
		}
		return n.Value()
	}

	switch {
	case isPointerType(n.Expr.Type()) && IsIntegerType(n.Index.Type()):
		switch v := n.Expr.eval(c, mode).(type) {
		case *UnknownValue:
			// nop
		case StringValue:
			switch x := n.Index.eval(c, 0).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				if x >= 0 && x < Int64Value(len(v)) {
					n.val = convert(Int64Value(v[x]), n.Type())
				}
			case UInt64Value:
				if x < UInt64Value(len(v)) {
					n.val = convert(Int64Value(v[x]), n.Type())
				}
			default:
				c.errors.add(errorf("TODO %T", x))
			}
		case UTF32StringValue:
			switch x := n.Index.eval(c, 0).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				if x >= 0 && x < Int64Value(len(v)) {
					n.val = convert(Int64Value(v[x]), n.Type())
				}
			case UInt64Value:
				if x < UInt64Value(len(v)) {
					n.val = convert(Int64Value(v[x]), n.Type())
				}
			default:
				c.errors.add(errorf("TODO %T", x))
			}
		case UTF16StringValue:
			switch x := n.Index.eval(c, 0).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				if x >= 0 && x < Int64Value(len(v)) {
					n.val = convert(Int64Value(v[x]), n.Type())
				}
			case UInt64Value:
				if x < UInt64Value(len(v)) {
					n.val = convert(Int64Value(v[x]), n.Type())
				}
			default:
				c.errors.add(errorf("TODO %T", x))
			}
		case UInt64Value:
			// nop
		default:
			// trc("%v: %v %v [%v %v] %T", n.Token.Position(), n.PostfixExpression.Value(), n.PostfixExpression.Type(), n.ExpressionList.Value(), n.ExpressionList.Type(), v)
			c.errors.add(errorf("TODO %T", v))
		}
	case IsIntegerType(n.Expr.Type()) && isPointerType(n.Index.Type()):
		switch v := n.Index.eval(c, mode).(type) {
		case *UnknownValue:
			// nop
		case StringValue:
			switch x := n.Expr.eval(c, 0).(type) {
			case *UnknownValue:
				// nop
			case Int64Value:
				if x >= 0 && x < Int64Value(len(v)) {
					n.val = convert(Int64Value(v[x]), n.Type())
				}
			case UInt64Value:
				if x < UInt64Value(len(v)) {
					n.val = convert(Int64Value(v[x]), n.Type())
				}
			default:
				c.errors.add(errorf("TODO %T", x))
			}
		default:
			// trc("%v: %v %v [%v %v] %T", n.Token.Position(), n.PostfixExpression.Value(), n.PostfixExpression.Type(), n.ExpressionList.Value(), n.ExpressionList.Type(), v)
			c.errors.add(errorf("TODO %T", v))
		}
	}

	return n.Value()
}

func (n *CallExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %T %v", n, mode.has(addrOf)))
	}
	// nop
	return n.Value()
}

func (n *SelectorExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		if !n.Ptr {
			switch x := n.Expr.Type().(type) {
			case *StructType:
				switch y := n.Expr.eval(c, mode).(type) {
				case *UnknownValue:
					// ok
				case UInt64Value:
					if f := x.FieldByName(n.Sel.SrcStr()); f != nil {
						n.val = convert(y+UInt64Value(f.Offset()), c.newPointerType(f.Type()))
					}
				default:
					c.errors.add(errorf("TODO %T %T", x, y))
				}
			case *UnionType:
				switch y := n.Expr.eval(c, mode).(type) {
				case *UnknownValue:
					// ok
				case UInt64Value:
					if f := x.FieldByName(n.Sel.SrcStr()); f != nil {
						n.val = convert(y+UInt64Value(f.Offset()), c.newPointerType(f.Type()))
					}
				default:
					c.errors.add(errorf("TODO %T %T", x, y))
				}
			default:
				c.errors.add(errorf("TODO %T", x))
			}
		} else {
			switch x := n.Expr.Type().(type) {
			case *PointerType:
				switch y := n.Expr.eval(c, mode.del(addrOf)).(type) {
				case *UnknownValue:
					// ok
				case UInt64Value:
					switch z := x.Elem().(type) {
					case *StructType:
						if f := z.FieldByName(n.Sel.SrcStr()); f != nil {
							n.val = convert(y+UInt64Value(f.Offset()), c.newPointerType(f.Type()))
						}
					case *UnionType:
						if f := z.FieldByName(n.Sel.SrcStr()); f != nil {
							n.val = convert(y+UInt64Value(f.Offset()), c.newPointerType(f.Type()))
						}
					default:
						c.errors.add(errorf("TODO %T %T", x, y, z))
					}
				default:
					c.errors.add(errorf("TODO %T %T", x, y))
				}
			default:
				c.errors.add(errorf("TODO %T", x))
			}
		}
		return n.Value()
	}

	// nop
	return n.Value()
}

func (n *PostfixExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %T %v", n, mode.has(addrOf)))
		return n.Value()
	}

	// nop
	return n.Value()
}

func (n *CompositeLitExpr) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		// ok
		return n.Value()
	}

	if n.InitializerList == nil || n.InitializerList.InitializerList != nil || n.InitializerList.Initializer.Case != InitializerExpr {
		return n.Value()
	}

	v := convert(n.InitializerList.Initializer.Expression.eval(c, mode), n.Type())
	switch n.Type().(type) {
	case *PredefinedType:
		n.val = v
	case *EnumType:
		n.val = v
	case *PointerType:
		n.val = v
	}
	return n.Value()
}

func (n *PrimaryExpression) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		switch n.Case {
		case PrimaryExpressionIdent: // IDENTIFIER
			// nop
		case PrimaryExpressionInt: // INTCONST
			c.errors.add(errorf("TODO %v %v", n.Case, mode.has(addrOf)))
		case PrimaryExpressionFloat: // FLOATCONST
			c.errors.add(errorf("TODO %v %v", n.Case, mode.has(addrOf)))
		case PrimaryExpressionChar: // CHARCONST
			c.errors.add(errorf("TODO %v %v", n.Case, mode.has(addrOf)))
		case PrimaryExpressionLChar: // LONGCHARCONST
			c.errors.add(errorf("TODO %v %v", n.Case, mode.has(addrOf)))
		case PrimaryExpressionString: // STRINGLITERAL
			// ok
		case PrimaryExpressionLString: // LONGSTRINGLITERAL
			c.errors.add(errorf("TODO %v %v", n.Case, mode.has(addrOf)))
		case PrimaryExpressionExpr: // '(' Expression ')'
			n.val = n.ExpressionList.eval(c, mode)
		case PrimaryExpressionStmt: // '(' CompoundStatement ')'
			c.errors.add(errorf("TODO %v %v", n.Case, mode.has(addrOf)))
		case PrimaryExpressionGeneric: // GenericSelection
			c.errors.add(errorf("TODO %v %v", n.Case, mode.has(addrOf)))
		default:
			c.errors.add(errorf("internal error: %v", n.Case))
		}
		return n.Value()
	}

	switch n.Case {
	case PrimaryExpressionIdent: // IDENTIFIER
		switch n.resolvedTo.(type) {
		case *Declarator, *Parameter, *Enumerator, nil:
			// ok
		default:
			c.errors.add(errorf("TODO %v %T", n.Case, n.resolvedTo))
		}
	case PrimaryExpressionInt: // INTCONST
		// nop
	case PrimaryExpressionFloat: // FLOATCONST
		// nop
	case PrimaryExpressionChar: // CHARCONST
		// nop
	case PrimaryExpressionLChar: // LONGCHARCONST
		// nop
	case PrimaryExpressionString: // STRINGLITERAL
		// nop
	case PrimaryExpressionLString: // LONGSTRINGLITERAL
		// nop
	case PrimaryExpressionExpr: // '(' Expression ')'
		n.val = n.ExpressionList.eval(c, mode)
	case PrimaryExpressionStmt: // '(' CompoundStatement ')'
		// nop
	case PrimaryExpressionGeneric: // GenericSelection
		// nop
	default:
		c.errors.add(errorf("internal error: %v", n.Case))
	}
	return n.Value()
}

func (n *ExpressionList) eval(c *ctx, mode flags) (r Value) {
	if len(n.List) == 0 {
		return Unknown
	}

	for _, n := range n.List {
		_ = n.Type()
		_ = n.eval(c, mode)
	}
	return n.List[len(n.List)-1].Value()
}

func (n *AssignmentExpression) eval(c *ctx, mode flags) (r Value) {
	if mode.has(addrOf) {
		c.errors.add(errorf("TODO %v %v", n.Op, mode.has(addrOf)))
		return n.Value()
	}

	switch n.Op {
	case AssignmentOperationAssign:
		n.val = convert(n.Rhs.eval(c, mode), n.Lhs.Type())
	case AssignmentOperationMul,
		AssignmentOperationDiv,
		AssignmentOperationMod,
		AssignmentOperationAdd,
		AssignmentOperationSub,
		AssignmentOperationLsh,
		AssignmentOperationRsh,
		AssignmentOperationAnd,
		AssignmentOperationXor,
		AssignmentOperationOr:

		n.Lhs.eval(c, mode)
		n.Rhs.eval(c, mode)
	default:
		c.errors.add(errorf("internal error: %v", n.Op))
	}
	return n.Value()
}

func isZero(v Value) bool {
	switch x := v.(type) {
	case *UnknownValue:
		return false
	case Int64Value:
		return x == 0
	case UInt64Value:
		return x == 0
	default:
		panic(todo("%T", x))
	}
}

func isNonzero(v Value) bool {
	switch x := v.(type) {
	case *UnknownValue:
		return false
	case Int64Value:
		return x != 0
	case UInt64Value:
		return x != 0
	default:
		panic(todo("%T", x))
	}
}
