// Copyright 2022 The CC Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cc

import (
	"fmt"
)

func ExampleAbstractDeclarator_ptr() {
	fmt.Println(exampleAST(195, "void f(int*);"))
	// Output:
	// &cc.AbstractDeclarator{
	// · Case: AbstractDeclaratorPtr,
	// · Pointer: &cc.Pointer{
	// · · Case: PointerTypeQual,
	// · · Token: example.c:1:11: '*' "*",
	// · },
	// }
}

func ExampleAbstractDeclarator_decl() {
	fmt.Println(exampleAST(196, "void f(int());"))
	// Output:
	// &cc.AbstractDeclarator{
	// · Case: AbstractDeclaratorDecl,
	// · DirectAbstractDeclarator: &cc.DirectAbstractDeclarator{
	// · · Case: DirectAbstractDeclaratorFunc,
	// · · Token: example.c:1:11: '(' "(",
	// · · Token2: example.c:1:12: ')' ")",
	// · },
	// }
}

func ExampleBinaryExpression_add() {
	fmt.Println(exampleAST(49, "int i = x+y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationAdd,
	// · Token: example.c:1:10: '+' "+",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_sub() {
	fmt.Println(exampleAST(50, "int i = x-y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationSub,
	// · Token: example.c:1:10: '-' "-",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "y",
	// · },
	// }
}

func ExampleAlignmentSpecifier_type() {
	fmt.Println(exampleAST(171, "_Alignas(double) char c;"))
	// Output:
	// &cc.AlignmentSpecifier{
	// · Case: AlignmentSpecifierType,
	// · Token: example.c:1:1: '_Alignas' "_Alignas",
	// · Token2: example.c:1:9: '(' "(",
	// · Token3: example.c:1:16: ')' ")",
	// · TypeName: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierDouble,
	// · · · · Name: example.c:1:10: 'double' "double",
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleAlignmentSpecifier_expr() {
	fmt.Println(exampleAST(172, "_Alignas(0ll) char c;"))
	// Output:
	// &cc.AlignmentSpecifier{
	// · Case: AlignmentSpecifierExpr,
	// · Expression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:10: integer constant "0ll",
	// · · },
	// · },
	// · Token: example.c:1:1: '_Alignas' "_Alignas",
	// · Token2: example.c:1:9: '(' "(",
	// · Token3: example.c:1:13: ')' ")",
	// }
}

func ExampleBinaryExpression_and() {
	fmt.Println(exampleAST(63, "int i = x & y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationAnd,
	// · Token: example.c:1:11: '&' "&",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:13: identifier "y",
	// · },
	// }
}

func ExampleArgumentExpressionList_case0() {
	fmt.Println(exampleAST(24, "int i = f(x);"))
	// Output:
	// &cc.ArgumentExpressionList{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// }
}

func ExampleArgumentExpressionList_case1() {
	fmt.Println(exampleAST(25, "int i = f(x, y);"))
	// Output:
	// &cc.ArgumentExpressionList{
	// · ArgumentExpressionList: &cc.ArgumentExpressionList{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:14: identifier "y",
	// · · },
	// · · Token: example.c:1:12: ',' ",",
	// · },
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// }
}

func ExampleAsm_case0() {
	fmt.Println(exampleAST(260, "__asm__(\"nop\");"))
	// Output:
	// &cc.Asm{
	// · Token: example.c:1:1: '__asm__' "__asm__",
	// · Token2: example.c:1:8: '(' "(",
	// · Token3: example.c:1:9: string literal "\"nop\"",
	// · Token4: example.c:1:14: ')' ")",
	// }
}

func ExampleAsmArgList_case0() {
	fmt.Println(exampleAST(258, "__asm__(\"nop\": a);"))
	// Output:
	// &cc.AsmArgList{
	// · AsmExpressionList: &cc.AsmExpressionList{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:16: identifier "a",
	// · · },
	// · },
	// · Token: example.c:1:14: ':' ":",
	// }
}

func ExampleAsmArgList_case1() {
	fmt.Println(exampleAST(259, "__asm__(\"nop\": a : b);"))
	// Output:
	// &cc.AsmArgList{
	// · AsmArgList: &cc.AsmArgList{
	// · · AsmExpressionList: &cc.AsmExpressionList{
	// · · · Expression: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:20: identifier "b",
	// · · · },
	// · · },
	// · · Token: example.c:1:18: ':' ":",
	// · },
	// · AsmExpressionList: &cc.AsmExpressionList{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:16: identifier "a",
	// · · },
	// · },
	// · Token: example.c:1:14: ':' ":",
	// }
}

func ExampleAsmExpressionList_case0() {
	fmt.Println(exampleAST(256, "__asm__(\"nop\": a);"))
	// Output:
	// &cc.AsmExpressionList{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "a",
	// · },
	// }
}

func ExampleAsmExpressionList_case1() {
	fmt.Println(exampleAST(257, "__asm__(\"nop\": a, b);"))
	// Output:
	// &cc.AsmExpressionList{
	// · AsmExpressionList: &cc.AsmExpressionList{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:19: identifier "b",
	// · · },
	// · · Token: example.c:1:17: ',' ",",
	// · },
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "a",
	// · },
	// }
}

func ExampleAsmIndex_case0() {
	fmt.Println(exampleAST(255, "__asm__(\"nop\": [a] b);"))
	// Output:
	// &cc.AsmIndex{
	// · ExpressionList: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:17: identifier "a",
	// · },
	// · Token: example.c:1:16: '[' "[",
	// · Token2: example.c:1:18: ']' "]",
	// }
}

func ExampleAsmQualifier_volatile() {
	fmt.Println(exampleAST(262, "__asm__ volatile (\"nop\");"))
	// Output:
	// &cc.AsmQualifier{
	// · Case: AsmQualifierVolatile,
	// · Token: example.c:1:9: 'volatile' "volatile",
	// }
}

func ExampleAsmQualifier_inline() {
	fmt.Println(exampleAST(263, "__asm__ inline (\"nop\");"))
	// Output:
	// &cc.AsmQualifier{
	// · Case: AsmQualifierInline,
	// · Token: example.c:1:9: 'inline' "inline",
	// }
}

func ExampleAsmQualifier_goto() {
	fmt.Println(exampleAST(264, "__asm__ goto (\"nop\");"))
	// Output:
	// &cc.AsmQualifier{
	// · Case: AsmQualifierGoto,
	// · Token: example.c:1:9: 'goto' "goto",
	// }
}

func ExampleAsmQualifierList_case0() {
	fmt.Println(exampleAST(265, "__asm__ inline (\"nop\");"))
	// Output:
	// &cc.AsmQualifierList{
	// · AsmQualifier: &cc.AsmQualifier{
	// · · Case: AsmQualifierInline,
	// · · Token: example.c:1:9: 'inline' "inline",
	// · },
	// }
}

func ExampleAsmQualifierList_case1() {
	fmt.Println(exampleAST(266, "__asm__ inline volatile (\"nop\");"))
	// Output:
	// &cc.AsmQualifierList{
	// · AsmQualifier: &cc.AsmQualifier{
	// · · Case: AsmQualifierInline,
	// · · Token: example.c:1:9: 'inline' "inline",
	// · },
	// · AsmQualifierList: &cc.AsmQualifierList{
	// · · AsmQualifier: &cc.AsmQualifier{
	// · · · Case: AsmQualifierVolatile,
	// · · · Token: example.c:1:16: 'volatile' "volatile",
	// · · },
	// · },
	// }
}

func ExampleAsmStatement_case0() {
	fmt.Println(exampleAST(261, "void f() { __asm__(\"nop\"); }"))
	// Output:
	// &cc.AsmStatement{
	// · Asm: &cc.Asm{
	// · · Token: example.c:1:12: '__asm__' "__asm__",
	// · · Token2: example.c:1:19: '(' "(",
	// · · Token3: example.c:1:20: string literal "\"nop\"",
	// · · Token4: example.c:1:25: ')' ")",
	// · },
	// · Token: example.c:1:26: ';' ";",
	// }
}

func ExampleAssignmentExpression_assign() {
	fmt.Println(exampleAST(75, "int f() { x = y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationAssign,
	// · Token: example.c:1:13: '=' "=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:15: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_mul() {
	fmt.Println(exampleAST(76, "int f() { x *= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationMul,
	// · Token: example.c:1:13: '*=' "*=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_div() {
	fmt.Println(exampleAST(77, "int f() { x /= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationDiv,
	// · Token: example.c:1:13: '/=' "/=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_mod() {
	fmt.Println(exampleAST(78, "int f() { x %= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationMod,
	// · Token: example.c:1:13: '%=' "%=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_add() {
	fmt.Println(exampleAST(79, "int f() { x += y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationAdd,
	// · Token: example.c:1:13: '+=' "+=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_sub() {
	fmt.Println(exampleAST(80, "int f() { x -= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationSub,
	// · Token: example.c:1:13: '-=' "-=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_lsh() {
	fmt.Println(exampleAST(81, "int f() { x <<= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationLsh,
	// · Token: example.c:1:13: '<<=' "<<=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:17: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_rsh() {
	fmt.Println(exampleAST(82, "int f() { x >>= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationRsh,
	// · Token: example.c:1:13: '>>=' ">>=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:17: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_and() {
	fmt.Println(exampleAST(83, "int f() { x &= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationAnd,
	// · Token: example.c:1:13: '&=' "&=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_xor() {
	fmt.Println(exampleAST(84, "int f() { x ^= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationXor,
	// · Token: example.c:1:13: '^=' "^=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "y",
	// · },
	// }
}

func ExampleAssignmentExpression_or() {
	fmt.Println(exampleAST(85, "int f() { x |= y; }"))
	// Output:
	// &cc.AssignmentExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Op: AssignmentOperationOr,
	// · Token: example.c:1:13: '|=' "|=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "y",
	// · },
	// }
}

func ExampleAtomicTypeSpecifier_case0() {
	fmt.Println(exampleAST(161, "_Atomic(int) i;"))
	// Output:
	// &cc.AtomicTypeSpecifier{
	// · Token: example.c:1:1: '_Atomic' "_Atomic",
	// · Token2: example.c:1:8: '(' "(",
	// · Token3: example.c:1:12: ')' ")",
	// · TypeName: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:9: 'int' "int",
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleAttributeSpecifier_case0() {
	fmt.Println(exampleAST(271, "int i __attribute__((a));"))
	// Output:
	// &cc.AttributeSpecifier{
	// · AttributeValueList: &cc.AttributeValueList{
	// · · AttributeValue: &cc.AttributeValue{
	// · · · Case: AttributeValueIdent,
	// · · · Token: example.c:1:22: identifier "a",
	// · · },
	// · },
	// · Token: example.c:1:7: '__attribute__' "__attribute__",
	// · Token2: example.c:1:20: '(' "(",
	// · Token3: example.c:1:21: '(' "(",
	// · Token4: example.c:1:23: ')' ")",
	// · Token5: example.c:1:24: ')' ")",
	// }
}

func ExampleAttributeSpecifier_case1() {
	fmt.Println(exampleAST(272, "int i __attribute__((a));"))
	// Output:
	// &cc.AttributeSpecifier{
	// · AttributeValueList: &cc.AttributeValueList{
	// · · AttributeValue: &cc.AttributeValue{
	// · · · Case: AttributeValueIdent,
	// · · · Token: example.c:1:22: identifier "a",
	// · · },
	// · },
	// · Token: example.c:1:7: '__attribute__' "__attribute__",
	// · Token2: example.c:1:20: '(' "(",
	// · Token3: example.c:1:21: '(' "(",
	// · Token4: example.c:1:23: ')' ")",
	// · Token5: example.c:1:24: ')' ")",
	// }
}

func ExampleAttributeSpecifier_case2() {
	fmt.Println(exampleAST(273, "int i __attribute__((a)) __attribute__((b));"))
	// Output:
	// &cc.AttributeSpecifier{
	// · AttributeValueList: &cc.AttributeValueList{
	// · · AttributeValue: &cc.AttributeValue{
	// · · · Case: AttributeValueIdent,
	// · · · Token: example.c:1:22: identifier "a",
	// · · },
	// · },
	// · Token: example.c:1:7: '__attribute__' "__attribute__",
	// · Token2: example.c:1:20: '(' "(",
	// · Token3: example.c:1:21: '(' "(",
	// · Token4: example.c:1:23: ')' ")",
	// · Token5: example.c:1:24: ')' ")",
	// }
	// &cc.AttributeSpecifier{
	// · AttributeValueList: &cc.AttributeValueList{
	// · · AttributeValue: &cc.AttributeValue{
	// · · · Case: AttributeValueIdent,
	// · · · Token: example.c:1:41: identifier "b",
	// · · },
	// · },
	// · Token: example.c:1:26: '__attribute__' "__attribute__",
	// · Token2: example.c:1:39: '(' "(",
	// · Token3: example.c:1:40: '(' "(",
	// · Token4: example.c:1:42: ')' ")",
	// · Token5: example.c:1:43: ')' ")",
	// }
}

func ExampleAttributeValue_ident() {
	fmt.Println(exampleAST(267, "int i __attribute__((a));"))
	// Output:
	// &cc.AttributeValue{
	// · Case: AttributeValueIdent,
	// · Token: example.c:1:22: identifier "a",
	// }
}

func ExampleAttributeValue_expr() {
	fmt.Println(exampleAST(268, "int i __attribute__((a(b)));"))
	// Output:
	// &cc.AttributeValue{
	// · ArgumentExpressionList: &cc.ArgumentExpressionList{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:24: identifier "b",
	// · · },
	// · },
	// · Case: AttributeValueExpr,
	// · Token: example.c:1:22: identifier "a",
	// · Token2: example.c:1:23: '(' "(",
	// · Token3: example.c:1:25: ')' ")",
	// }
}

func ExampleAttributeValueList_case0() {
	fmt.Println(exampleAST(269, "int i __attribute__((a));"))
	// Output:
	// &cc.AttributeValueList{
	// · AttributeValue: &cc.AttributeValue{
	// · · Case: AttributeValueIdent,
	// · · Token: example.c:1:22: identifier "a",
	// · },
	// }
}

func ExampleAttributeValueList_case1() {
	fmt.Println(exampleAST(270, "int i __attribute__((a, b));"))
	// Output:
	// &cc.AttributeValueList{
	// · AttributeValue: &cc.AttributeValue{
	// · · Case: AttributeValueIdent,
	// · · Token: example.c:1:22: identifier "a",
	// · },
	// · AttributeValueList: &cc.AttributeValueList{
	// · · AttributeValue: &cc.AttributeValue{
	// · · · Case: AttributeValueIdent,
	// · · · Token: example.c:1:25: identifier "b",
	// · · },
	// · · Token: example.c:1:23: ',' ",",
	// · },
	// }
}

func ExampleBlockItem_decl() {
	fmt.Println(exampleAST(229, "int f() { int i; }"))
	// Output:
	// &cc.FunctionDefinition{
	// · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorFuncParam,
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:5: identifier "f",
	// · · · },
	// · · · Token: example.c:1:6: '(' "(",
	// · · · Token2: example.c:1:7: ')' ")",
	// · · },
	// · },
	// · Body: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:9: '{' "{",
	// · · List: []*cc.CommonDeclaration{ // len 2
	// · · · 0: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · · },
	// · · · · · 1: &cc.TypeQualifier{
	// · · · · · · Case: TypeQualifierConst,
	// · · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · · },
	// · · · · · 2: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · · },
	// · · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · · },
	// · · · · · · · Case: InitializerExpr,
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '=' "=",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:9: ';' ";",
	// · · · },
	// · · · 1: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierInt,
	// · · · · · · Name: example.c:1:11: 'int' "int",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:15: identifier "i",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:16: ';' ";",
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:18: '}' "}",
	// · },
	// }
}

func ExampleBlockItem_label() {
	fmt.Println(exampleAST(230, "int f() { __label__ L; int i; }"))
	// Output:
	// &cc.FunctionDefinition{
	// · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorFuncParam,
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:5: identifier "f",
	// · · · },
	// · · · Token: example.c:1:6: '(' "(",
	// · · · Token2: example.c:1:7: ')' ")",
	// · · },
	// · },
	// · Body: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:9: '{' "{",
	// · · List: []*cc.CommonDeclaration{ // len 3
	// · · · 0: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · · },
	// · · · · · 1: &cc.TypeQualifier{
	// · · · · · · Case: TypeQualifierConst,
	// · · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · · },
	// · · · · · 2: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · · },
	// · · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · · },
	// · · · · · · · Case: InitializerExpr,
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '=' "=",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:9: ';' ";",
	// · · · },
	// · · · 1: &cc.LabelDeclaration{
	// · · · · IdentifierList: &cc.IdentifierList{
	// · · · · · Token2: example.c:1:21: identifier "L",
	// · · · · },
	// · · · · Token: example.c:1:11: '__label__' "__label__",
	// · · · · Token2: example.c:1:22: ';' ";",
	// · · · },
	// · · · 2: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierInt,
	// · · · · · · Name: example.c:1:24: 'int' "int",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:28: identifier "i",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:29: ';' ";",
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:31: '}' "}",
	// · },
	// }
}

func ExampleBlockItem_stmt() {
	fmt.Println(exampleAST(231, "int f() { g(); }"))
	// Output:
	// &cc.FunctionDefinition{
	// · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorFuncParam,
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:5: identifier "f",
	// · · · },
	// · · · Token: example.c:1:6: '(' "(",
	// · · · Token2: example.c:1:7: ')' ")",
	// · · },
	// · },
	// · Body: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:9: '{' "{",
	// · · List: []*cc.CommonDeclaration{ // len 2
	// · · · 0: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · · },
	// · · · · · 1: &cc.TypeQualifier{
	// · · · · · · Case: TypeQualifierConst,
	// · · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · · },
	// · · · · · 2: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · · },
	// · · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · · },
	// · · · · · · · Case: InitializerExpr,
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '=' "=",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:9: ';' ";",
	// · · · },
	// · · · 1: &cc.ExpressionStatement{
	// · · · · ExpressionList: &cc.CallExpr{
	// · · · · · Func: &cc.PrimaryExpression{
	// · · · · · · Case: PrimaryExpressionIdent,
	// · · · · · · Token: example.c:1:11: identifier "g",
	// · · · · · },
	// · · · · · LeftParen: example.c:1:12: '(' "(",
	// · · · · · RightParen: example.c:1:13: ')' ")",
	// · · · · },
	// · · · · Token: example.c:1:14: ';' ";",
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:16: '}' "}",
	// · },
	// }
}

func ExampleBlockItem_funcDef() {
	fmt.Println(exampleAST(232, "int f() { int g() {} }"))
	// Output:
	// &cc.FunctionDefinition{
	// · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorFuncParam,
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:5: identifier "f",
	// · · · },
	// · · · Token: example.c:1:6: '(' "(",
	// · · · Token2: example.c:1:7: ')' ")",
	// · · },
	// · },
	// · Body: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:9: '{' "{",
	// · · List: []*cc.CommonDeclaration{ // len 2
	// · · · 0: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · · },
	// · · · · · 1: &cc.TypeQualifier{
	// · · · · · · Case: TypeQualifierConst,
	// · · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · · },
	// · · · · · 2: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · · },
	// · · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · · },
	// · · · · · · · Case: InitializerExpr,
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '=' "=",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:9: ';' ";",
	// · · · },
	// · · · 1: &cc.FunctionDefinition{
	// · · · · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierInt,
	// · · · · · · Name: example.c:1:11: 'int' "int",
	// · · · · · },
	// · · · · },
	// · · · · Declarator: &cc.Declarator{
	// · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · Case: DirectDeclaratorFuncParam,
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · Token: example.c:1:15: identifier "g",
	// · · · · · · },
	// · · · · · · Token: example.c:1:16: '(' "(",
	// · · · · · · Token2: example.c:1:17: ')' ")",
	// · · · · · },
	// · · · · },
	// · · · · Body: &cc.CompoundStatement{
	// · · · · · Lbrace: example.c:1:19: '{' "{",
	// · · · · · List: []*cc.CommonDeclaration{ // len 1
	// · · · · · · 0: &cc.CommonDeclaration{
	// · · · · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · · · · Token: example.c:1:19: 'static' "static",
	// · · · · · · · · },
	// · · · · · · · · 1: &cc.TypeQualifier{
	// · · · · · · · · · Case: TypeQualifierConst,
	// · · · · · · · · · Token: example.c:1:19: 'const' "const",
	// · · · · · · · · },
	// · · · · · · · · 2: &cc.TypeSpecName{
	// · · · · · · · · · Case: TypeSpecifierChar,
	// · · · · · · · · · Name: example.c:1:19: 'char' "char",
	// · · · · · · · · },
	// · · · · · · · },
	// · · · · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · · · · 0: &cc.InitDeclarator{
	// · · · · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · · · · Token: example.c:1:19: identifier "__func__",
	// · · · · · · · · · · · },
	// · · · · · · · · · · · Token: example.c:1:19: '[' "[",
	// · · · · · · · · · · · Token2: example.c:1:19: ']' "]",
	// · · · · · · · · · · },
	// · · · · · · · · · },
	// · · · · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · · · · Token: example.c:1:19: string literal "\"g\"",
	// · · · · · · · · · · },
	// · · · · · · · · · · Case: InitializerExpr,
	// · · · · · · · · · },
	// · · · · · · · · · Token: example.c:1:19: '=' "=",
	// · · · · · · · · },
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:19: ';' ";",
	// · · · · · · },
	// · · · · · },
	// · · · · · Rbrace: example.c:1:20: '}' "}",
	// · · · · },
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:22: '}' "}",
	// · },
	// }
}

func ExampleBlockItem_case0() {
	fmt.Println(exampleAST(227, "int f() { int i; }"))
	// Output:
	// &cc.FunctionDefinition{
	// · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorFuncParam,
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:5: identifier "f",
	// · · · },
	// · · · Token: example.c:1:6: '(' "(",
	// · · · Token2: example.c:1:7: ')' ")",
	// · · },
	// · },
	// · Body: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:9: '{' "{",
	// · · List: []*cc.CommonDeclaration{ // len 2
	// · · · 0: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · · },
	// · · · · · 1: &cc.TypeQualifier{
	// · · · · · · Case: TypeQualifierConst,
	// · · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · · },
	// · · · · · 2: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · · },
	// · · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · · },
	// · · · · · · · Case: InitializerExpr,
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '=' "=",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:9: ';' ";",
	// · · · },
	// · · · 1: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierInt,
	// · · · · · · Name: example.c:1:11: 'int' "int",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:15: identifier "i",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:16: ';' ";",
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:18: '}' "}",
	// · },
	// }
}

func ExampleBlockItem_case1() {
	fmt.Println(exampleAST(228, "int f() { int i; double j; }"))
	// Output:
	// &cc.FunctionDefinition{
	// · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorFuncParam,
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:5: identifier "f",
	// · · · },
	// · · · Token: example.c:1:6: '(' "(",
	// · · · Token2: example.c:1:7: ')' ")",
	// · · },
	// · },
	// · Body: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:9: '{' "{",
	// · · List: []*cc.CommonDeclaration{ // len 3
	// · · · 0: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · · },
	// · · · · · 1: &cc.TypeQualifier{
	// · · · · · · Case: TypeQualifierConst,
	// · · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · · },
	// · · · · · 2: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · · },
	// · · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · · },
	// · · · · · · · Case: InitializerExpr,
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '=' "=",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:9: ';' ";",
	// · · · },
	// · · · 1: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierInt,
	// · · · · · · Name: example.c:1:11: 'int' "int",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:15: identifier "i",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:16: ';' ";",
	// · · · },
	// · · · 2: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierDouble,
	// · · · · · · Name: example.c:1:18: 'double' "double",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:25: identifier "j",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:26: ';' ";",
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:28: '}' "}",
	// · },
	// }
}

func ExampleCastExpr_cast() {
	fmt.Println(exampleAST(43, "int i = (__attribute__((a)) int)3.14;"))
	// Output:
	// &cc.CastExpr{
	// · Lparen: example.c:1:9: '(' "(",
	// · TypeName: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeQualifier{ // len 2
	// · · · 0: &cc.TypeQualifier{
	// · · · · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · · · · 0: &cc.AttributeSpecifier{
	// · · · · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · · · · Case: AttributeValueIdent,
	// · · · · · · · · Token: example.c:1:25: identifier "a",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Token: example.c:1:10: '__attribute__' "__attribute__",
	// · · · · · · Token2: example.c:1:23: '(' "(",
	// · · · · · · Token3: example.c:1:24: '(' "(",
	// · · · · · · Token4: example.c:1:26: ')' ")",
	// · · · · · · Token5: example.c:1:27: ')' ")",
	// · · · · · },
	// · · · · },
	// · · · · Case: TypeQualifierAttr,
	// · · · },
	// · · · 1: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:29: 'int' "int",
	// · · · },
	// · · },
	// · },
	// · Rparen: example.c:1:32: ')' ")",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionFloat,
	// · · Token: example.c:1:33: floating point constant "3.14",
	// · },
	// }
}

func ExampleCompoundStatement_case0() {
	fmt.Println(exampleAST(225, "int f() { __label__ L; int i; }"))
	// Output:
	// &cc.CompoundStatement{
	// · Lbrace: example.c:1:9: '{' "{",
	// · List: []*cc.CommonDeclaration{ // len 3
	// · · 0: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · },
	// · · · · 1: &cc.TypeQualifier{
	// · · · · · Case: TypeQualifierConst,
	// · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · },
	// · · · · 2: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierChar,
	// · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · },
	// · · · · · },
	// · · · · · Initializer: &cc.Initializer{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · },
	// · · · · · · Case: InitializerExpr,
	// · · · · · },
	// · · · · · Token: example.c:1:9: '=' "=",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:9: ';' ";",
	// · · },
	// · · 1: &cc.LabelDeclaration{
	// · · · IdentifierList: &cc.IdentifierList{
	// · · · · Token2: example.c:1:21: identifier "L",
	// · · · },
	// · · · Token: example.c:1:11: '__label__' "__label__",
	// · · · Token2: example.c:1:22: ';' ";",
	// · · },
	// · · 2: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierInt,
	// · · · · · Name: example.c:1:24: 'int' "int",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · Token: example.c:1:28: identifier "i",
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:29: ';' ";",
	// · · },
	// · },
	// · Rbrace: example.c:1:31: '}' "}",
	// }
}

func ExampleConditionalExpression_cond() {
	fmt.Println(exampleAST(73, "int i = x ? y : z;"))
	// Output:
	// &cc.ConditionalExpression{
	// · Condition: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Token: example.c:1:11: '?' "?",
	// · Then: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:13: identifier "y",
	// · },
	// · Token2: example.c:1:15: ':' ":",
	// · Else: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:17: identifier "z",
	// · },
	// }
}

func ExampleConstantExpression_case0() {
	fmt.Println(exampleAST(88, "struct { int i:3; };"))
	// Output:
	// &cc.ConstantExpression{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionInt,
	// · · Token: example.c:1:16: integer constant "3",
	// · },
	// }
}

func ExampleDeclaration_decl() {
	fmt.Println(exampleAST(89, "int i, j __attribute__((a));"))
	// Output:
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 2
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:5: identifier "i",
	// · · · · },
	// · · · },
	// · · · Token2: example.c:1:6: ',' ",",
	// · · },
	// · · 1: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:8: identifier "j",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:28: ';' ";",
	// }
}

func ExampleDeclaration_assert() {
	fmt.Println(exampleAST(90, "_Static_assert(x > y, \"abc\")"))
	// Output:
	// &cc.StaticAssertDeclaration{
	// · Expression: &cc.ConstantExpression{
	// · · Expression: &cc.BinaryExpression{
	// · · · Lhs: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:16: identifier "x",
	// · · · },
	// · · · Op: BinaryOperationGt,
	// · · · Token: example.c:1:18: '>' ">",
	// · · · Rhs: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:20: identifier "y",
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:1: _Static_assert "_Static_assert",
	// · Token2: example.c:1:15: '(' "(",
	// · Token3: example.c:1:21: ',' ",",
	// · Token4: example.c:1:23: string literal "\"abc\"",
	// · Token5: example.c:1:28: ')' ")",
	// }
}

func ExampleDeclaration_auto() {
	fmt.Println(exampleAST(91, "__auto_type x = y;"))
	// Output:
	// &cc.AutoDeclaration{
	// · Token: example.c:1:1: '__auto_type' "__auto_type",
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:13: identifier "x",
	// · · },
	// · },
	// · Token2: example.c:1:15: '=' "=",
	// · Initializer: &cc.Initializer{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:17: identifier "y",
	// · · },
	// · · Case: InitializerExpr,
	// · },
	// · Token3: example.c:1:18: ';' ";",
	// }
}

func ExampleDeclaration_case0() {
	fmt.Println(exampleAST(253, "int f(i) int i; {}"))
	// Output:
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:10: 'int' "int",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:14: identifier "i",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:15: ';' ";",
	// }
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · 0: &cc.StorageClassSpecifier{
	// · · · Case: StorageClassSpecifierStatic,
	// · · · Token: example.c:1:17: 'static' "static",
	// · · },
	// · · 1: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:17: 'const' "const",
	// · · },
	// · · 2: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierChar,
	// · · · Name: example.c:1:17: 'char' "char",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorArr,
	// · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · Token: example.c:1:17: identifier "__func__",
	// · · · · · },
	// · · · · · Token: example.c:1:17: '[' "[",
	// · · · · · Token2: example.c:1:17: ']' "]",
	// · · · · },
	// · · · },
	// · · · Initializer: &cc.Initializer{
	// · · · · Expression: &cc.PrimaryExpression{
	// · · · · · Case: PrimaryExpressionString,
	// · · · · · Token: example.c:1:17: string literal "\"f\"",
	// · · · · },
	// · · · · Case: InitializerExpr,
	// · · · },
	// · · · Token: example.c:1:17: '=' "=",
	// · · },
	// · },
	// · Token: example.c:1:17: ';' ";",
	// }
}

func ExampleDeclaration_case1() {
	fmt.Println(exampleAST(254, "int f(i, j) int i; int j; {}"))
	// Output:
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:13: 'int' "int",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:17: identifier "i",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:18: ';' ";",
	// }
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:20: 'int' "int",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:24: identifier "j",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:25: ';' ";",
	// }
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · 0: &cc.StorageClassSpecifier{
	// · · · Case: StorageClassSpecifierStatic,
	// · · · Token: example.c:1:27: 'static' "static",
	// · · },
	// · · 1: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:27: 'const' "const",
	// · · },
	// · · 2: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierChar,
	// · · · Name: example.c:1:27: 'char' "char",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorArr,
	// · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · Token: example.c:1:27: identifier "__func__",
	// · · · · · },
	// · · · · · Token: example.c:1:27: '[' "[",
	// · · · · · Token2: example.c:1:27: ']' "]",
	// · · · · },
	// · · · },
	// · · · Initializer: &cc.Initializer{
	// · · · · Expression: &cc.PrimaryExpression{
	// · · · · · Case: PrimaryExpressionString,
	// · · · · · Token: example.c:1:27: string literal "\"f\"",
	// · · · · },
	// · · · · Case: InitializerExpr,
	// · · · },
	// · · · Token: example.c:1:27: '=' "=",
	// · · },
	// · },
	// · Token: example.c:1:27: ';' ";",
	// }
}

func ExampleDeclarationSpecifier_storage() {
	fmt.Println(exampleAST(93, "__attribute__((a)) static int i;"))
	// Output:
	// &cc.AttributeSpecifier{
	// · AttributeValueList: &cc.AttributeValueList{
	// · · AttributeValue: &cc.AttributeValue{
	// · · · Case: AttributeValueIdent,
	// · · · Token: example.c:1:16: identifier "a",
	// · · },
	// · },
	// · Token: example.c:1:1: '__attribute__' "__attribute__",
	// · Token2: example.c:1:14: '(' "(",
	// · Token3: example.c:1:15: '(' "(",
	// · Token4: example.c:1:17: ')' ")",
	// · Token5: example.c:1:18: ')' ")",
	// }
	// &cc.StorageClassSpecifier{
	// · Case: StorageClassSpecifierStatic,
	// · Token: example.c:1:20: 'static' "static",
	// }
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:27: 'int' "int",
	// }
}

func ExampleDeclarationSpecifier_typeSpec() {
	fmt.Println(exampleAST(94, "int i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:1: 'int' "int",
	// }
}

func ExampleDeclarationSpecifier_typeQual() {
	fmt.Println(exampleAST(95, "volatile int i;"))
	// Output:
	// &cc.TypeQualifier{
	// · Case: TypeQualifierVolatile,
	// · Token: example.c:1:1: 'volatile' "volatile",
	// }
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:10: 'int' "int",
	// }
}

func ExampleDeclarationSpecifier_func() {
	fmt.Println(exampleAST(96, "inline int f() {}"))
	// Output:
	// &cc.FunctionSpecifier{
	// · Case: FunctionSpecifierInline,
	// · Token: example.c:1:1: 'inline' "inline",
	// }
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:8: 'int' "int",
	// }
	// &cc.StorageClassSpecifier{
	// · Case: StorageClassSpecifierStatic,
	// · Token: example.c:1:16: 'static' "static",
	// }
	// &cc.TypeQualifier{
	// · Case: TypeQualifierConst,
	// · Token: example.c:1:16: 'const' "const",
	// }
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierChar,
	// · Name: example.c:1:16: 'char' "char",
	// }
}

func ExampleDeclarationSpecifier_alignSpec() {
	fmt.Println(exampleAST(97, "_Alignas(double) int i;"))
	// Output:
	// &cc.AlignmentSpecifier{
	// · Case: AlignmentSpecifierType,
	// · Token: example.c:1:1: '_Alignas' "_Alignas",
	// · Token2: example.c:1:9: '(' "(",
	// · Token3: example.c:1:16: ')' ")",
	// · TypeName: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierDouble,
	// · · · · Name: example.c:1:10: 'double' "double",
	// · · · },
	// · · },
	// · },
	// }
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:18: 'int' "int",
	// }
}

func ExampleDeclarationSpecifier_attr() {
	fmt.Println(exampleAST(98, "__attribute__((a)) int i;"))
	// Output:
	// &cc.AttributeSpecifier{
	// · AttributeValueList: &cc.AttributeValueList{
	// · · AttributeValue: &cc.AttributeValue{
	// · · · Case: AttributeValueIdent,
	// · · · Token: example.c:1:16: identifier "a",
	// · · },
	// · },
	// · Token: example.c:1:1: '__attribute__' "__attribute__",
	// · Token2: example.c:1:14: '(' "(",
	// · Token3: example.c:1:15: '(' "(",
	// · Token4: example.c:1:17: ')' ")",
	// · Token5: example.c:1:18: ')' ")",
	// }
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:20: 'int' "int",
	// }
}

func ExampleDeclarator_case0() {
	fmt.Println(exampleAST(170, "int *p;"))
	// Output:
	// &cc.Declarator{
	// · DirectDeclarator: &cc.DirectDeclarator{
	// · · Case: DirectDeclaratorIdent,
	// · · Token: example.c:1:6: identifier "p",
	// · },
	// · Pointer: &cc.Pointer{
	// · · Case: PointerTypeQual,
	// · · Token: example.c:1:5: '*' "*",
	// · },
	// }
}

func ExampleDesignation_case0() {
	fmt.Println(exampleAST(207, "int a[] = { [42] = 314 };"))
	// Output:
	// &cc.Designation{
	// · Designators: []*cc.Designator{ // len 1
	// · · 0: &cc.Designator{
	// · · · Case: DesignatorIndex,
	// · · · ConstantExpression: &cc.ConstantExpression{
	// · · · · Expression: &cc.PrimaryExpression{
	// · · · · · Case: PrimaryExpressionInt,
	// · · · · · Token: example.c:1:14: integer constant "42",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:13: '[' "[",
	// · · · Token2: example.c:1:16: ']' "]",
	// · · },
	// · },
	// · Token: example.c:1:18: '=' "=",
	// }
}

func ExampleDesignator_index() {
	fmt.Println(exampleAST(210, "int a[] = { [42] = 314 };"))
	// Output:
	// &cc.Designator{
	// · Case: DesignatorIndex,
	// · ConstantExpression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:14: integer constant "42",
	// · · },
	// · },
	// · Token: example.c:1:13: '[' "[",
	// · Token2: example.c:1:16: ']' "]",
	// }
}

func ExampleDesignator_index2() {
	fmt.Println(exampleAST(211, "int a[] = { [42 ... 278] = 314 };"))
	// Output:
	// &cc.Designator{
	// · Case: DesignatorIndex2,
	// · ConstantExpression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:14: integer constant "42",
	// · · },
	// · },
	// · ConstantExpression2: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:21: integer constant "278",
	// · · },
	// · },
	// · Token: example.c:1:13: '[' "[",
	// · Token2: example.c:1:17: '...' "...",
	// · Token3: example.c:1:24: ']' "]",
	// }
}

func ExampleDesignator_field() {
	fmt.Println(exampleAST(212, "struct t s = { .fld = 314 };"))
	// Output:
	// &cc.Designator{
	// · Case: DesignatorField,
	// · Token: example.c:1:16: '.' ".",
	// · Token2: example.c:1:17: identifier "fld",
	// }
}

func ExampleDesignator_field2() {
	fmt.Println(exampleAST(213, "struct t s = { fld: 314 };"))
	// Output:
	// &cc.Designator{
	// · Case: DesignatorField2,
	// · Token: example.c:1:16: identifier "fld",
	// · Token2: example.c:1:19: ':' ":",
	// }
}

func ExampleDesignator_case0() {
	fmt.Println(exampleAST(208, "int a[] = { [42] = 314 };"))
	// Output:
	// &cc.Designator{
	// · Case: DesignatorIndex,
	// · ConstantExpression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:14: integer constant "42",
	// · · },
	// · },
	// · Token: example.c:1:13: '[' "[",
	// · Token2: example.c:1:16: ']' "]",
	// }
}

func ExampleDesignator_case1() {
	fmt.Println(exampleAST(209, "int a[100][] = { [42][12] = 314 };"))
	// Output:
	// &cc.Designator{
	// · Case: DesignatorIndex,
	// · ConstantExpression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:19: integer constant "42",
	// · · },
	// · },
	// · Token: example.c:1:18: '[' "[",
	// · Token2: example.c:1:21: ']' "]",
	// }
	// &cc.Designator{
	// · Case: DesignatorIndex,
	// · ConstantExpression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:23: integer constant "12",
	// · · },
	// · },
	// · Token: example.c:1:22: '[' "[",
	// · Token2: example.c:1:25: ']' "]",
	// }
}

func ExampleDirectAbstractDeclarator_decl() {
	fmt.Println(exampleAST(197, "void f(int(*));"))
	// Output:
	// &cc.DirectAbstractDeclarator{
	// · AbstractDeclarator: &cc.AbstractDeclarator{
	// · · Case: AbstractDeclaratorPtr,
	// · · Pointer: &cc.Pointer{
	// · · · Case: PointerTypeQual,
	// · · · Token: example.c:1:12: '*' "*",
	// · · },
	// · },
	// · Case: DirectAbstractDeclaratorDecl,
	// · Token: example.c:1:11: '(' "(",
	// · Token2: example.c:1:13: ')' ")",
	// }
}

func ExampleDirectAbstractDeclarator_arr() {
	fmt.Println(exampleAST(198, "void f(int[const 42]);"))
	// Output:
	// &cc.DirectAbstractDeclarator{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionInt,
	// · · Token: example.c:1:18: integer constant "42",
	// · },
	// · Case: DirectAbstractDeclaratorArr,
	// · Token: example.c:1:11: '[' "[",
	// · Token2: example.c:1:20: ']' "]",
	// · TypeQualifiers: []*cc.TypeQualifier{ // len 1
	// · · 0: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:12: 'const' "const",
	// · · },
	// · },
	// }
}

func ExampleDirectAbstractDeclarator_staticArr() {
	fmt.Println(exampleAST(199, "void f(int[static const 42]);"))
	// Output:
	// &cc.DirectAbstractDeclarator{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionInt,
	// · · Token: example.c:1:25: integer constant "42",
	// · },
	// · Case: DirectAbstractDeclaratorStaticArr,
	// · Token: example.c:1:11: '[' "[",
	// · Token2: example.c:1:12: 'static' "static",
	// · Token3: example.c:1:27: ']' "]",
	// · TypeQualifiers: []*cc.TypeQualifier{ // len 1
	// · · 0: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:19: 'const' "const",
	// · · },
	// · },
	// }
}

func ExampleDirectAbstractDeclarator_arrStatic() {
	fmt.Println(exampleAST(200, "void f(int[const static 42]);"))
	// Output:
	// &cc.DirectAbstractDeclarator{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionInt,
	// · · Token: example.c:1:25: integer constant "42",
	// · },
	// · Case: DirectAbstractDeclaratorArr,
	// · Token: example.c:1:11: '[' "[",
	// · Token2: example.c:1:18: 'static' "static",
	// · Token3: example.c:1:27: ']' "]",
	// · TypeQualifiers: []*cc.TypeQualifier{ // len 1
	// · · 0: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:12: 'const' "const",
	// · · },
	// · },
	// }
}

func ExampleDirectAbstractDeclarator_arrStar() {
	fmt.Println(exampleAST(201, "void f(int[*]);"))
	// Output:
	// &cc.DirectAbstractDeclarator{
	// · Case: DirectAbstractDeclaratorArrStar,
	// · Token: example.c:1:11: '[' "[",
	// · Token2: example.c:1:12: '*' "*",
	// · Token3: example.c:1:13: ']' "]",
	// }
}

func ExampleDirectAbstractDeclarator_func() {
	fmt.Println(exampleAST(202, "void f(int(char));"))
	// Output:
	// &cc.DirectAbstractDeclarator{
	// · Case: DirectAbstractDeclaratorFunc,
	// · ParameterTypeList: &cc.ParameterTypeList{
	// · · Case: ParameterTypeListList,
	// · · ParameterList: &cc.ParameterList{
	// · · · ParameterDeclaration: &cc.ParameterDeclaration{
	// · · · · Case: ParameterDeclarationAbstract,
	// · · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:12: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:11: '(' "(",
	// · Token2: example.c:1:16: ')' ")",
	// }
}

func ExampleDirectDeclarator_ident() {
	fmt.Println(exampleAST(173, "int i;"))
	// Output:
	// &cc.DirectDeclarator{
	// · Case: DirectDeclaratorIdent,
	// · Token: example.c:1:5: identifier "i",
	// }
}

func ExampleDirectDeclarator_decl() {
	fmt.Println(exampleAST(174, "int (f);"))
	// Output:
	// &cc.DirectDeclarator{
	// · Case: DirectDeclaratorDecl,
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:6: identifier "f",
	// · · },
	// · },
	// · Token: example.c:1:5: '(' "(",
	// · Token2: example.c:1:7: ')' ")",
	// }
}

func ExampleDirectDeclarator_arr() {
	fmt.Println(exampleAST(175, "int i[const 42];"))
	// Output:
	// &cc.DirectDeclarator{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionInt,
	// · · Token: example.c:1:13: integer constant "42",
	// · },
	// · Case: DirectDeclaratorArr,
	// · DirectDeclarator: &cc.DirectDeclarator{
	// · · Case: DirectDeclaratorIdent,
	// · · Token: example.c:1:5: identifier "i",
	// · },
	// · Token: example.c:1:6: '[' "[",
	// · Token3: example.c:1:15: ']' "]",
	// · TypeQualifiers: []*cc.TypeQualifier{ // len 1
	// · · 0: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:7: 'const' "const",
	// · · },
	// · },
	// }
}

func ExampleDirectDeclarator_staticArr() {
	fmt.Println(exampleAST(176, "int i[static const 42];"))
	// Output:
	// &cc.DirectDeclarator{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionInt,
	// · · Token: example.c:1:20: integer constant "42",
	// · },
	// · Case: DirectDeclaratorStaticArr,
	// · DirectDeclarator: &cc.DirectDeclarator{
	// · · Case: DirectDeclaratorIdent,
	// · · Token: example.c:1:5: identifier "i",
	// · },
	// · Token: example.c:1:6: '[' "[",
	// · Token2: example.c:1:7: 'static' "static",
	// · Token3: example.c:1:22: ']' "]",
	// · TypeQualifiers: []*cc.TypeQualifier{ // len 1
	// · · 0: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:14: 'const' "const",
	// · · },
	// · },
	// }
}

func ExampleDirectDeclarator_arrStatic() {
	fmt.Println(exampleAST(177, "int i[const static 42];"))
	// Output:
	// &cc.DirectDeclarator{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionInt,
	// · · Token: example.c:1:20: integer constant "42",
	// · },
	// · Case: DirectDeclaratorArrStatic,
	// · DirectDeclarator: &cc.DirectDeclarator{
	// · · Case: DirectDeclaratorIdent,
	// · · Token: example.c:1:5: identifier "i",
	// · },
	// · Token: example.c:1:6: '[' "[",
	// · Token2: example.c:1:13: 'static' "static",
	// · Token3: example.c:1:22: ']' "]",
	// · TypeQualifiers: []*cc.TypeQualifier{ // len 1
	// · · 0: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:7: 'const' "const",
	// · · },
	// · },
	// }
}

func ExampleDirectDeclarator_star() {
	fmt.Println(exampleAST(178, "int i[const *];"))
	// Output:
	// &cc.DirectDeclarator{
	// · Case: DirectDeclaratorStar,
	// · DirectDeclarator: &cc.DirectDeclarator{
	// · · Case: DirectDeclaratorIdent,
	// · · Token: example.c:1:5: identifier "i",
	// · },
	// · Token: example.c:1:6: '[' "[",
	// · Token2: example.c:1:13: '*' "*",
	// · Token3: example.c:1:14: ']' "]",
	// · TypeQualifiers: []*cc.TypeQualifier{ // len 1
	// · · 0: &cc.TypeQualifier{
	// · · · Case: TypeQualifierConst,
	// · · · Token: example.c:1:7: 'const' "const",
	// · · },
	// · },
	// }
}

func ExampleDirectDeclarator_funcParam() {
	fmt.Println(exampleAST(179, "int f(int i);"))
	// Output:
	// &cc.DirectDeclarator{
	// · Case: DirectDeclaratorFuncParam,
	// · DirectDeclarator: &cc.DirectDeclarator{
	// · · Case: DirectDeclaratorIdent,
	// · · Token: example.c:1:5: identifier "f",
	// · },
	// · ParameterTypeList: &cc.ParameterTypeList{
	// · · Case: ParameterTypeListList,
	// · · ParameterList: &cc.ParameterList{
	// · · · ParameterDeclaration: &cc.ParameterDeclaration{
	// · · · · Case: ParameterDeclarationDecl,
	// · · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierInt,
	// · · · · · · Name: example.c:1:7: 'int' "int",
	// · · · · · },
	// · · · · },
	// · · · · Declarator: &cc.Declarator{
	// · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · Token: example.c:1:11: identifier "i",
	// · · · · · },
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:6: '(' "(",
	// · Token2: example.c:1:12: ')' ")",
	// }
}

func ExampleDirectDeclarator_funcIdent() {
	fmt.Println(exampleAST(180, "int f(a);"))
	// Output:
	// &cc.DirectDeclarator{
	// · Case: DirectDeclaratorFuncIdent,
	// · DirectDeclarator: &cc.DirectDeclarator{
	// · · Case: DirectDeclaratorIdent,
	// · · Token: example.c:1:5: identifier "f",
	// · },
	// · IdentifierList: &cc.IdentifierList{
	// · · Token2: example.c:1:7: identifier "a",
	// · },
	// · Token: example.c:1:6: '(' "(",
	// · Token2: example.c:1:8: ')' ")",
	// }
}

func ExampleEnumSpecifier_def() {
	fmt.Println(exampleAST(155, "enum e {a};"))
	// Output:
	// &cc.EnumSpecifier{
	// · Case: EnumSpecifierDef,
	// · EnumeratorList: &cc.EnumeratorList{
	// · · Enumerator: &cc.Enumerator{
	// · · · Case: EnumeratorIdent,
	// · · · Token: example.c:1:9: identifier "a",
	// · · },
	// · },
	// · Token: example.c:1:1: 'enum' "enum",
	// · Token2: example.c:1:6: identifier "e",
	// · Token3: example.c:1:8: '{' "{",
	// · Token5: example.c:1:10: '}' "}",
	// }
}

func ExampleEnumSpecifier_tag() {
	fmt.Println(exampleAST(156, "enum e i;"))
	// Output:
	// &cc.EnumSpecifier{
	// · Case: EnumSpecifierTag,
	// · Token: example.c:1:1: 'enum' "enum",
	// · Token2: example.c:1:6: identifier "e",
	// }
}

func ExampleEnumerator_ident() {
	fmt.Println(exampleAST(159, "enum e {a};"))
	// Output:
	// &cc.Enumerator{
	// · Case: EnumeratorIdent,
	// · Token: example.c:1:9: identifier "a",
	// }
}

func ExampleEnumerator_expr() {
	fmt.Println(exampleAST(160, "enum e {a = 42};"))
	// Output:
	// &cc.Enumerator{
	// · Case: EnumeratorExpr,
	// · Expression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:13: integer constant "42",
	// · · },
	// · },
	// · Token: example.c:1:9: identifier "a",
	// · Token2: example.c:1:11: '=' "=",
	// }
}

func ExampleEnumeratorList_case0() {
	fmt.Println(exampleAST(157, "enum e {a};"))
	// Output:
	// &cc.EnumeratorList{
	// · Enumerator: &cc.Enumerator{
	// · · Case: EnumeratorIdent,
	// · · Token: example.c:1:9: identifier "a",
	// · },
	// }
}

func ExampleEnumeratorList_case1() {
	fmt.Println(exampleAST(158, "enum e {a, b};"))
	// Output:
	// &cc.EnumeratorList{
	// · Enumerator: &cc.Enumerator{
	// · · Case: EnumeratorIdent,
	// · · Token: example.c:1:9: identifier "a",
	// · },
	// · EnumeratorList: &cc.EnumeratorList{
	// · · Enumerator: &cc.Enumerator{
	// · · · Case: EnumeratorIdent,
	// · · · Token: example.c:1:12: identifier "b",
	// · · },
	// · · Token: example.c:1:10: ',' ",",
	// · },
	// }
}

func ExampleBinaryExpression_eq() {
	fmt.Println(exampleAST(60, "int i = x == y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationEq,
	// · Token: example.c:1:11: '==' "==",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_neq() {
	fmt.Println(exampleAST(61, "int i = x != y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationNeq,
	// · Token: example.c:1:11: '!=' "!=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_xor() {
	fmt.Println(exampleAST(65, "int i = x^y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationXor,
	// · Token: example.c:1:10: '^' "^",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "y",
	// · },
	// }
}

func ExampleExpressionList_assign() {
	fmt.Println(exampleAST(86, "int f() { i = x; };"))
	// Output:
	// <nil>
}

func ExampleExpressionList_comma() {
	fmt.Println(exampleAST(87, "int f() { x, y; };"))
	// Output:
	// &cc.ExpressionList{
	// · List: []*cc.PrimaryExpression{ // len 2
	// · · 0: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:11: identifier "x",
	// · · },
	// · · 1: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:14: identifier "y",
	// · · },
	// · },
	// · Tokens: []cc.Token{ // len 1
	// · · 0: example.c:1:12: ',' ",",
	// · },
	// }
}

func ExampleExpressionStatement_case0() {
	fmt.Println(exampleAST(233, "int f() { g(); }"))
	// Output:
	// &cc.ExpressionStatement{
	// · ExpressionList: &cc.CallExpr{
	// · · Func: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:11: identifier "g",
	// · · },
	// · · LeftParen: example.c:1:12: '(' "(",
	// · · RightParen: example.c:1:13: ')' ")",
	// · },
	// · Token: example.c:1:14: ';' ";",
	// }
}

func ExampleExternalDeclaration_funcDef() {
	fmt.Println(exampleAST(248, "int f() {}"))
	// Output:
	// &cc.FunctionDefinition{
	// · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorFuncParam,
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:5: identifier "f",
	// · · · },
	// · · · Token: example.c:1:6: '(' "(",
	// · · · Token2: example.c:1:7: ')' ")",
	// · · },
	// · },
	// · Body: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:9: '{' "{",
	// · · List: []*cc.CommonDeclaration{ // len 1
	// · · · 0: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · · },
	// · · · · · 1: &cc.TypeQualifier{
	// · · · · · · Case: TypeQualifierConst,
	// · · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · · },
	// · · · · · 2: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · · },
	// · · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · · },
	// · · · · · · · Case: InitializerExpr,
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '=' "=",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:9: ';' ";",
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:10: '}' "}",
	// · },
	// }
}

func ExampleExternalDeclaration_decl() {
	fmt.Println(exampleAST(249, "register int i __asm__(\"r0\");"))
	// Output:
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 2
	// · · 0: &cc.StorageClassSpecifier{
	// · · · Case: StorageClassSpecifierRegister,
	// · · · Token: example.c:1:1: 'register' "register",
	// · · },
	// · · 1: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:10: 'int' "int",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Asm: &cc.Asm{
	// · · · · Token: example.c:1:16: '__asm__' "__asm__",
	// · · · · Token2: example.c:1:23: '(' "(",
	// · · · · Token3: example.c:1:24: string literal "\"r0\"",
	// · · · · Token4: example.c:1:28: ')' ")",
	// · · · },
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:14: identifier "i",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:29: ';' ";",
	// }
}

func ExampleExternalDeclaration_asmStmt() {
	fmt.Println(exampleAST(250, "__asm__(\"nop\");"))
	// Output:
	// &cc.AsmStatement{
	// · Asm: &cc.Asm{
	// · · Token: example.c:1:1: '__asm__' "__asm__",
	// · · Token2: example.c:1:8: '(' "(",
	// · · Token3: example.c:1:9: string literal "\"nop\"",
	// · · Token4: example.c:1:14: ')' ")",
	// · },
	// · Token: example.c:1:15: ';' ";",
	// }
}

func ExampleExternalDeclaration_empty() {
	fmt.Println(exampleAST(251, ";"))
	// Output:
	// &cc.CommonDeclaration{
	// · Token: example.c:1:1: ';' ";",
	// }
}

func ExampleFunctionDefinition_case0() {
	fmt.Println(exampleAST(252, "int f() {}"))
	// Output:
	// &cc.FunctionDefinition{
	// · Specifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorFuncParam,
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:5: identifier "f",
	// · · · },
	// · · · Token: example.c:1:6: '(' "(",
	// · · · Token2: example.c:1:7: ')' ")",
	// · · },
	// · },
	// · Body: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:9: '{' "{",
	// · · List: []*cc.CommonDeclaration{ // len 1
	// · · · 0: &cc.CommonDeclaration{
	// · · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · · },
	// · · · · · 1: &cc.TypeQualifier{
	// · · · · · · Case: TypeQualifierConst,
	// · · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · · },
	// · · · · · 2: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierChar,
	// · · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · · },
	// · · · · },
	// · · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · · 0: &cc.InitDeclarator{
	// · · · · · · Declarator: &cc.Declarator{
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · · },
	// · · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Initializer: &cc.Initializer{
	// · · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · · },
	// · · · · · · · Case: InitializerExpr,
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '=' "=",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:9: ';' ";",
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:10: '}' "}",
	// · },
	// }
}

func ExampleFunctionSpecifier_inline() {
	fmt.Println(exampleAST(168, "inline int f() {}"))
	// Output:
	// &cc.FunctionSpecifier{
	// · Case: FunctionSpecifierInline,
	// · Token: example.c:1:1: 'inline' "inline",
	// }
}

func ExampleFunctionSpecifier_noreturn() {
	fmt.Println(exampleAST(169, "_Noreturn int f() {}"))
	// Output:
	// &cc.FunctionSpecifier{
	// · Case: FunctionSpecifierNoreturn,
	// · Token: example.c:1:1: '_Noreturn' "_Noreturn",
	// }
}

func ExampleGenericAssociation_type() {
	fmt.Println(exampleAST(14, "int i = _Generic(x, int: y)(42);"))
	// Output:
	// &cc.GenericAssociation{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:26: identifier "y",
	// · },
	// · Case: GenericAssociationType,
	// · Token: example.c:1:24: ':' ":",
	// · TypeName: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:21: 'int' "int",
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleGenericAssociation_default() {
	fmt.Println(exampleAST(15, "int i = _Generic(x, default: y)(42);"))
	// Output:
	// &cc.GenericAssociation{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:30: identifier "y",
	// · },
	// · Case: GenericAssociationDefault,
	// · Token: example.c:1:21: 'default' "default",
	// · Token2: example.c:1:28: ':' ":",
	// }
}

func ExampleGenericAssociationList_case0() {
	fmt.Println(exampleAST(12, "int i = _Generic(x, int: y)(42);"))
	// Output:
	// &cc.GenericAssociationList{
	// · GenericAssociation: &cc.GenericAssociation{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:26: identifier "y",
	// · · },
	// · · Case: GenericAssociationType,
	// · · Token: example.c:1:24: ':' ":",
	// · · TypeName: &cc.TypeName{
	// · · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierInt,
	// · · · · · Name: example.c:1:21: 'int' "int",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleGenericAssociationList_case1() {
	fmt.Println(exampleAST(13, "int i = _Generic(x, int: y, float: z)(42);"))
	// Output:
	// &cc.GenericAssociationList{
	// · GenericAssociation: &cc.GenericAssociation{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:26: identifier "y",
	// · · },
	// · · Case: GenericAssociationType,
	// · · Token: example.c:1:24: ':' ":",
	// · · TypeName: &cc.TypeName{
	// · · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierInt,
	// · · · · · Name: example.c:1:21: 'int' "int",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · GenericAssociationList: &cc.GenericAssociationList{
	// · · GenericAssociation: &cc.GenericAssociation{
	// · · · Expression: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:36: identifier "z",
	// · · · },
	// · · · Case: GenericAssociationType,
	// · · · Token: example.c:1:34: ':' ":",
	// · · · TypeName: &cc.TypeName{
	// · · · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierFloat,
	// · · · · · · Name: example.c:1:29: 'float' "float",
	// · · · · · },
	// · · · · },
	// · · · },
	// · · },
	// · · Token: example.c:1:27: ',' ",",
	// · },
	// }
}

func ExampleGenericSelection_case0() {
	fmt.Println(exampleAST(11, "int i = _Generic(x, int: y)(42);"))
	// Output:
	// &cc.GenericSelection{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:18: identifier "x",
	// · },
	// · GenericAssociationList: &cc.GenericAssociationList{
	// · · GenericAssociation: &cc.GenericAssociation{
	// · · · Expression: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:26: identifier "y",
	// · · · },
	// · · · Case: GenericAssociationType,
	// · · · Token: example.c:1:24: ':' ":",
	// · · · TypeName: &cc.TypeName{
	// · · · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · 0: &cc.TypeSpecName{
	// · · · · · · Case: TypeSpecifierInt,
	// · · · · · · Name: example.c:1:21: 'int' "int",
	// · · · · · },
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:9: '_Generic' "_Generic",
	// · Token2: example.c:1:17: '(' "(",
	// · Token3: example.c:1:19: ',' ",",
	// · Token4: example.c:1:27: ')' ")",
	// }
}

func ExampleIdentifierList_case0() {
	fmt.Println(exampleAST(192, "int f(i) int i; {}"))
	// Output:
	// &cc.IdentifierList{
	// · Token2: example.c:1:7: identifier "i",
	// }
}

func ExampleIdentifierList_case1() {
	fmt.Println(exampleAST(193, "int f(i, j) int i, j; {}"))
	// Output:
	// &cc.IdentifierList{
	// · IdentifierList: &cc.IdentifierList{
	// · · Token: example.c:1:8: ',' ",",
	// · · Token2: example.c:1:10: identifier "j",
	// · },
	// · Token2: example.c:1:7: identifier "i",
	// }
}

func ExampleBinaryExpression_or() {
	fmt.Println(exampleAST(67, "int i = x|y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationOr,
	// · Token: example.c:1:10: '|' "|",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "y",
	// · },
	// }
}

func ExampleInitDeclarator_decl() {
	fmt.Println(exampleAST(101, "register int i __asm__(\"r0\");"))
	// Output:
	// &cc.InitDeclarator{
	// · Asm: &cc.Asm{
	// · · Token: example.c:1:16: '__asm__' "__asm__",
	// · · Token2: example.c:1:23: '(' "(",
	// · · Token3: example.c:1:24: string literal "\"r0\"",
	// · · Token4: example.c:1:28: ')' ")",
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:14: identifier "i",
	// · · },
	// · },
	// }
}

func ExampleInitDeclarator_init() {
	fmt.Println(exampleAST(102, "register int i __asm__(\"r0\") = x;"))
	// Output:
	// &cc.InitDeclarator{
	// · Asm: &cc.Asm{
	// · · Token: example.c:1:16: '__asm__' "__asm__",
	// · · Token2: example.c:1:23: '(' "(",
	// · · Token3: example.c:1:24: string literal "\"r0\"",
	// · · Token4: example.c:1:28: ')' ")",
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:14: identifier "i",
	// · · },
	// · },
	// · Initializer: &cc.Initializer{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:32: identifier "x",
	// · · },
	// · · Case: InitializerExpr,
	// · },
	// · Token: example.c:1:30: '=' "=",
	// }
}

func ExampleInitDeclarator_case0() {
	fmt.Println(exampleAST(99, "int i;"))
	// Output:
	// &cc.InitDeclarator{
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:5: identifier "i",
	// · · },
	// · },
	// }
}

func ExampleInitDeclarator_case1() {
	fmt.Println(exampleAST(100, "int i, __attribute__((a)) j;"))
	// Output:
	// &cc.InitDeclarator{
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:5: identifier "i",
	// · · },
	// · },
	// · Token2: example.c:1:6: ',' ",",
	// }
	// &cc.InitDeclarator{
	// · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · 0: &cc.AttributeSpecifier{
	// · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · Case: AttributeValueIdent,
	// · · · · · Token: example.c:1:23: identifier "a",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:8: '__attribute__' "__attribute__",
	// · · · Token2: example.c:1:21: '(' "(",
	// · · · Token3: example.c:1:22: '(' "(",
	// · · · Token4: example.c:1:24: ')' ")",
	// · · · Token5: example.c:1:25: ')' ")",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:27: identifier "j",
	// · · },
	// · },
	// }
}

func ExampleInitializer_expr() {
	fmt.Println(exampleAST(203, "int i = x;"))
	// Output:
	// &cc.Initializer{
	// · Expression: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Case: InitializerExpr,
	// }
}

func ExampleInitializer_initList() {
	fmt.Println(exampleAST(204, "int i[] = { x };"))
	// Output:
	// &cc.Initializer{
	// · Case: InitializerInitList,
	// · InitializerList: &cc.InitializerList{
	// · · Initializer: &cc.Initializer{
	// · · · Expression: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:13: identifier "x",
	// · · · },
	// · · · Case: InitializerExpr,
	// · · },
	// · },
	// · Token: example.c:1:11: '{' "{",
	// · Token3: example.c:1:15: '}' "}",
	// }
}

func ExampleInitializerList_case0() {
	fmt.Println(exampleAST(205, "int i[] = { [10] = x };"))
	// Output:
	// &cc.InitializerList{
	// · Designation: &cc.Designation{
	// · · Designators: []*cc.Designator{ // len 1
	// · · · 0: &cc.Designator{
	// · · · · Case: DesignatorIndex,
	// · · · · ConstantExpression: &cc.ConstantExpression{
	// · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · Case: PrimaryExpressionInt,
	// · · · · · · Token: example.c:1:14: integer constant "10",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:13: '[' "[",
	// · · · · Token2: example.c:1:16: ']' "]",
	// · · · },
	// · · },
	// · · Token: example.c:1:18: '=' "=",
	// · },
	// · Initializer: &cc.Initializer{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:20: identifier "x",
	// · · },
	// · · Case: InitializerExpr,
	// · },
	// }
}

func ExampleInitializerList_case1() {
	fmt.Println(exampleAST(206, "int i[] = { [10] = x, [20] = y };"))
	// Output:
	// &cc.InitializerList{
	// · Designation: &cc.Designation{
	// · · Designators: []*cc.Designator{ // len 1
	// · · · 0: &cc.Designator{
	// · · · · Case: DesignatorIndex,
	// · · · · ConstantExpression: &cc.ConstantExpression{
	// · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · Case: PrimaryExpressionInt,
	// · · · · · · Token: example.c:1:14: integer constant "10",
	// · · · · · },
	// · · · · },
	// · · · · Token: example.c:1:13: '[' "[",
	// · · · · Token2: example.c:1:16: ']' "]",
	// · · · },
	// · · },
	// · · Token: example.c:1:18: '=' "=",
	// · },
	// · Initializer: &cc.Initializer{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:20: identifier "x",
	// · · },
	// · · Case: InitializerExpr,
	// · },
	// · InitializerList: &cc.InitializerList{
	// · · Designation: &cc.Designation{
	// · · · Designators: []*cc.Designator{ // len 1
	// · · · · 0: &cc.Designator{
	// · · · · · Case: DesignatorIndex,
	// · · · · · ConstantExpression: &cc.ConstantExpression{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionInt,
	// · · · · · · · Token: example.c:1:24: integer constant "20",
	// · · · · · · },
	// · · · · · },
	// · · · · · Token: example.c:1:23: '[' "[",
	// · · · · · Token2: example.c:1:26: ']' "]",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:28: '=' "=",
	// · · },
	// · · Initializer: &cc.Initializer{
	// · · · Expression: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:30: identifier "y",
	// · · · },
	// · · · Case: InitializerExpr,
	// · · },
	// · · Token: example.c:1:21: ',' ",",
	// · },
	// }
}

func ExampleIterationStatement_while() {
	fmt.Println(exampleAST(237, "int f() { while(x) y(); }"))
	// Output:
	// &cc.IterationStatement{
	// · Case: IterationStatementWhile,
	// · ExpressionList: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:17: identifier "x",
	// · },
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:20: identifier "y",
	// · · · },
	// · · · LeftParen: example.c:1:21: '(' "(",
	// · · · RightParen: example.c:1:22: ')' ")",
	// · · },
	// · · Token: example.c:1:23: ';' ";",
	// · },
	// · Token: example.c:1:11: 'while' "while",
	// · Token2: example.c:1:16: '(' "(",
	// · Token3: example.c:1:18: ')' ")",
	// }
}

func ExampleIterationStatement_do() {
	fmt.Println(exampleAST(238, "int f() { do x(); while(y); }"))
	// Output:
	// &cc.IterationStatement{
	// · Case: IterationStatementDo,
	// · ExpressionList: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:25: identifier "y",
	// · },
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:14: identifier "x",
	// · · · },
	// · · · LeftParen: example.c:1:15: '(' "(",
	// · · · RightParen: example.c:1:16: ')' ")",
	// · · },
	// · · Token: example.c:1:17: ';' ";",
	// · },
	// · Token: example.c:1:11: 'do' "do",
	// · Token2: example.c:1:19: 'while' "while",
	// · Token3: example.c:1:24: '(' "(",
	// · Token4: example.c:1:26: ')' ")",
	// · Token5: example.c:1:27: ';' ";",
	// }
}

func ExampleIterationStatement_for() {
	fmt.Println(exampleAST(239, "int f() { for( i = 0; i < 10; i++) x(); }"))
	// Output:
	// &cc.IterationStatement{
	// · Case: IterationStatementFor,
	// · ExpressionList: &cc.AssignmentExpression{
	// · · Lhs: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:16: identifier "i",
	// · · },
	// · · Op: AssignmentOperationAssign,
	// · · Token: example.c:1:18: '=' "=",
	// · · Rhs: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:20: integer constant "0",
	// · · },
	// · },
	// · ExpressionList2: &cc.BinaryExpression{
	// · · Lhs: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:23: identifier "i",
	// · · },
	// · · Op: BinaryOperationLt,
	// · · Token: example.c:1:25: '<' "<",
	// · · Rhs: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:27: integer constant "10",
	// · · },
	// · },
	// · ExpressionList3: &cc.PostfixExpr{
	// · · Expr: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:31: identifier "i",
	// · · },
	// · · Token: example.c:1:32: '++' "++",
	// · },
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:36: identifier "x",
	// · · · },
	// · · · LeftParen: example.c:1:37: '(' "(",
	// · · · RightParen: example.c:1:38: ')' ")",
	// · · },
	// · · Token: example.c:1:39: ';' ";",
	// · },
	// · Token: example.c:1:11: 'for' "for",
	// · Token2: example.c:1:14: '(' "(",
	// · Token3: example.c:1:21: ';' ";",
	// · Token4: example.c:1:29: ';' ";",
	// · Token5: example.c:1:34: ')' ")",
	// }
}

func ExampleIterationStatement_forDecl() {
	fmt.Println(exampleAST(240, "int f() { for( int i = 0; i < 10; i++) x(); }"))
	// Output:
	// &cc.IterationStatement{
	// · Case: IterationStatementForDecl,
	// · Declaration: &cc.CommonDeclaration{
	// · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:16: 'int' "int",
	// · · · },
	// · · },
	// · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · 0: &cc.InitDeclarator{
	// · · · · Declarator: &cc.Declarator{
	// · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · Token: example.c:1:20: identifier "i",
	// · · · · · },
	// · · · · },
	// · · · · Initializer: &cc.Initializer{
	// · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · Case: PrimaryExpressionInt,
	// · · · · · · Token: example.c:1:24: integer constant "0",
	// · · · · · },
	// · · · · · Case: InitializerExpr,
	// · · · · },
	// · · · · Token: example.c:1:22: '=' "=",
	// · · · },
	// · · },
	// · · Token: example.c:1:25: ';' ";",
	// · },
	// · ExpressionList: &cc.BinaryExpression{
	// · · Lhs: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:27: identifier "i",
	// · · },
	// · · Op: BinaryOperationLt,
	// · · Token: example.c:1:29: '<' "<",
	// · · Rhs: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:31: integer constant "10",
	// · · },
	// · },
	// · ExpressionList2: &cc.PostfixExpr{
	// · · Expr: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:35: identifier "i",
	// · · },
	// · · Token: example.c:1:36: '++' "++",
	// · },
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:40: identifier "x",
	// · · · },
	// · · · LeftParen: example.c:1:41: '(' "(",
	// · · · RightParen: example.c:1:42: ')' ")",
	// · · },
	// · · Token: example.c:1:43: ';' ";",
	// · },
	// · Token: example.c:1:11: 'for' "for",
	// · Token2: example.c:1:14: '(' "(",
	// · Token3: example.c:1:33: ';' ";",
	// · Token4: example.c:1:38: ')' ")",
	// }
}

func ExampleJumpStatement_goto() {
	fmt.Println(exampleAST(241, "int f() { L: goto L; }"))
	// Output:
	// &cc.JumpStatement{
	// · Case: JumpStatementGoto,
	// · Token: example.c:1:14: 'goto' "goto",
	// · Token2: example.c:1:19: identifier "L",
	// · Token3: example.c:1:20: ';' ";",
	// }
}

func ExampleJumpStatement_gotoExpr() {
	fmt.Println(exampleAST(242, "int f() { L: x(); void *p = &&L; goto *p; }"))
	// Output:
	// &cc.JumpStatement{
	// · Case: JumpStatementGotoExpr,
	// · ExpressionList: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:40: identifier "p",
	// · },
	// · Token: example.c:1:34: 'goto' "goto",
	// · Token2: example.c:1:39: '*' "*",
	// · Token3: example.c:1:41: ';' ";",
	// }
}

func ExampleJumpStatement_continue() {
	fmt.Println(exampleAST(243, "int f() { for(;;) if (i) continue; }"))
	// Output:
	// &cc.JumpStatement{
	// · Case: JumpStatementContinue,
	// · Token: example.c:1:26: 'continue' "continue",
	// · Token2: example.c:1:34: ';' ";",
	// }
}

func ExampleJumpStatement_break() {
	fmt.Println(exampleAST(244, "int f() { for(;;) if (i) break; }"))
	// Output:
	// &cc.JumpStatement{
	// · Case: JumpStatementBreak,
	// · Token: example.c:1:26: 'break' "break",
	// · Token2: example.c:1:31: ';' ";",
	// }
}

func ExampleJumpStatement_return() {
	fmt.Println(exampleAST(245, "int f() { if (i) return x; }"))
	// Output:
	// &cc.JumpStatement{
	// · Case: JumpStatementReturn,
	// · ExpressionList: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:25: identifier "x",
	// · },
	// · Token: example.c:1:18: 'return' "return",
	// · Token2: example.c:1:26: ';' ";",
	// }
}

func ExampleLabelDeclaration_case0() {
	fmt.Println(exampleAST(226, "int f() { __label__ L, M; L: x(); M: y(); }"))
	// Output:
	// &cc.LabelDeclaration{
	// · IdentifierList: &cc.IdentifierList{
	// · · IdentifierList: &cc.IdentifierList{
	// · · · Token: example.c:1:22: ',' ",",
	// · · · Token2: example.c:1:24: identifier "M",
	// · · },
	// · · Token2: example.c:1:21: identifier "L",
	// · },
	// · Token: example.c:1:11: '__label__' "__label__",
	// · Token2: example.c:1:25: ';' ";",
	// }
}

func ExampleLabeledStatement_label() {
	fmt.Println(exampleAST(221, "int f() { L: goto L; }"))
	// Output:
	// &cc.LabeledStatement{
	// · Case: LabeledStatementLabel,
	// · Statement: &cc.JumpStatement{
	// · · Case: JumpStatementGoto,
	// · · Token: example.c:1:14: 'goto' "goto",
	// · · Token2: example.c:1:19: identifier "L",
	// · · Token3: example.c:1:20: ';' ";",
	// · },
	// · Token: example.c:1:11: identifier "L",
	// · Token2: example.c:1:12: ':' ":",
	// }
}

func ExampleLabeledStatement_caseLabel() {
	fmt.Println(exampleAST(222, "int f() { switch(i) case 42: x(); }"))
	// Output:
	// &cc.LabeledStatement{
	// · Case: LabeledStatementCaseLabel,
	// · Expression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:26: integer constant "42",
	// · · },
	// · },
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:30: identifier "x",
	// · · · },
	// · · · LeftParen: example.c:1:31: '(' "(",
	// · · · RightParen: example.c:1:32: ')' ")",
	// · · },
	// · · Token: example.c:1:33: ';' ";",
	// · },
	// · Token: example.c:1:21: 'case' "case",
	// · Token2: example.c:1:28: ':' ":",
	// }
}

func ExampleLabeledStatement_range() {
	fmt.Println(exampleAST(223, "int f() { switch(i) case 42 ... 56: x(); }"))
	// Output:
	// &cc.LabeledStatement{
	// · Case: LabeledStatementRange,
	// · Expression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:26: integer constant "42",
	// · · },
	// · },
	// · Expression2: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:33: integer constant "56",
	// · · },
	// · },
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:37: identifier "x",
	// · · · },
	// · · · LeftParen: example.c:1:38: '(' "(",
	// · · · RightParen: example.c:1:39: ')' ")",
	// · · },
	// · · Token: example.c:1:40: ';' ";",
	// · },
	// · Token: example.c:1:21: 'case' "case",
	// · Token2: example.c:1:29: '...' "...",
	// · Token3: example.c:1:35: ':' ":",
	// }
}

func ExampleLabeledStatement_default() {
	fmt.Println(exampleAST(224, "int f() { switch(i) default: x(); }"))
	// Output:
	// &cc.LabeledStatement{
	// · Case: LabeledStatementDefault,
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:30: identifier "x",
	// · · · },
	// · · · LeftParen: example.c:1:31: '(' "(",
	// · · · RightParen: example.c:1:32: ')' ")",
	// · · },
	// · · Token: example.c:1:33: ';' ";",
	// · },
	// · Token: example.c:1:21: 'default' "default",
	// · Token2: example.c:1:28: ':' ":",
	// }
}

func ExampleBinaryExpression_lAnd() {
	fmt.Println(exampleAST(69, "int i = x && y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationLAnd,
	// · Token: example.c:1:11: '&&' "&&",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_lOr() {
	fmt.Println(exampleAST(71, "int i = x || y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationLOr,
	// · Token: example.c:1:11: '||' "||",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_mul() {
	fmt.Println(exampleAST(45, "int i = x * y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationMul,
	// · Token: example.c:1:11: '*' "*",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:13: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_div() {
	fmt.Println(exampleAST(46, "int i = x / y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationDiv,
	// · Token: example.c:1:11: '/' "/",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:13: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_mod() {
	fmt.Println(exampleAST(47, "int i = x % y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationMod,
	// · Token: example.c:1:11: '%' "%",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:13: identifier "y",
	// · },
	// }
}

func ExampleParameterDeclaration_decl() {
	fmt.Println(exampleAST(190, "int f(int i __attribute__((a))) {}"))
	// Output:
	// &cc.ParameterDeclaration{
	// · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · 0: &cc.AttributeSpecifier{
	// · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · Case: AttributeValueIdent,
	// · · · · · Token: example.c:1:28: identifier "a",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:13: '__attribute__' "__attribute__",
	// · · · Token2: example.c:1:26: '(' "(",
	// · · · Token3: example.c:1:27: '(' "(",
	// · · · Token4: example.c:1:29: ')' ")",
	// · · · Token5: example.c:1:30: ')' ")",
	// · · },
	// · },
	// · Case: ParameterDeclarationDecl,
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:7: 'int' "int",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:11: identifier "i",
	// · · },
	// · },
	// }
}

func ExampleParameterDeclaration_abstract() {
	fmt.Println(exampleAST(191, "int f(int*) {}"))
	// Output:
	// &cc.ParameterDeclaration{
	// · AbstractDeclarator: &cc.AbstractDeclarator{
	// · · Case: AbstractDeclaratorPtr,
	// · · Pointer: &cc.Pointer{
	// · · · Case: PointerTypeQual,
	// · · · Token: example.c:1:10: '*' "*",
	// · · },
	// · },
	// · Case: ParameterDeclarationAbstract,
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:7: 'int' "int",
	// · · },
	// · },
	// }
}

func ExampleParameterList_case0() {
	fmt.Println(exampleAST(188, "int f(int i) {}"))
	// Output:
	// &cc.ParameterList{
	// · ParameterDeclaration: &cc.ParameterDeclaration{
	// · · Case: ParameterDeclarationDecl,
	// · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:7: 'int' "int",
	// · · · },
	// · · },
	// · · Declarator: &cc.Declarator{
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:11: identifier "i",
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleParameterList_case1() {
	fmt.Println(exampleAST(189, "int f(int i, int j) {}"))
	// Output:
	// &cc.ParameterList{
	// · ParameterDeclaration: &cc.ParameterDeclaration{
	// · · Case: ParameterDeclarationDecl,
	// · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:7: 'int' "int",
	// · · · },
	// · · },
	// · · Declarator: &cc.Declarator{
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:11: identifier "i",
	// · · · },
	// · · },
	// · },
	// · ParameterList: &cc.ParameterList{
	// · · ParameterDeclaration: &cc.ParameterDeclaration{
	// · · · Case: ParameterDeclarationDecl,
	// · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierInt,
	// · · · · · Name: example.c:1:14: 'int' "int",
	// · · · · },
	// · · · },
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:18: identifier "j",
	// · · · · },
	// · · · },
	// · · },
	// · · Token: example.c:1:12: ',' ",",
	// · },
	// }
}

func ExampleParameterTypeList_list() {
	fmt.Println(exampleAST(186, "int f(int i) {}"))
	// Output:
	// &cc.ParameterTypeList{
	// · Case: ParameterTypeListList,
	// · ParameterList: &cc.ParameterList{
	// · · ParameterDeclaration: &cc.ParameterDeclaration{
	// · · · Case: ParameterDeclarationDecl,
	// · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierInt,
	// · · · · · Name: example.c:1:7: 'int' "int",
	// · · · · },
	// · · · },
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:11: identifier "i",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleParameterTypeList_var() {
	fmt.Println(exampleAST(187, "int f(int i, ...) {}"))
	// Output:
	// &cc.ParameterTypeList{
	// · Case: ParameterTypeListVar,
	// · ParameterList: &cc.ParameterList{
	// · · ParameterDeclaration: &cc.ParameterDeclaration{
	// · · · Case: ParameterDeclarationDecl,
	// · · · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierInt,
	// · · · · · Name: example.c:1:7: 'int' "int",
	// · · · · },
	// · · · },
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:11: identifier "i",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:12: ',' ",",
	// · Token2: example.c:1:14: '...' "...",
	// }
}

func ExamplePointer_typeQual() {
	fmt.Println(exampleAST(181, "int *p;"))
	// Output:
	// &cc.Pointer{
	// · Case: PointerTypeQual,
	// · Token: example.c:1:5: '*' "*",
	// }
}

func ExamplePointer_ptr() {
	fmt.Println(exampleAST(182, "int **p;"))
	// Output:
	// &cc.Pointer{
	// · Case: PointerPtr,
	// · Pointer: &cc.Pointer{
	// · · Case: PointerTypeQual,
	// · · Token: example.c:1:6: '*' "*",
	// · },
	// · Token: example.c:1:5: '*' "*",
	// }
}

func ExamplePointer_block() {
	fmt.Println(exampleAST(183, "int atexit_b(void (^ _Nonnull)(void));"))
	// Output:
	// &cc.Pointer{
	// · Case: PointerBlock,
	// · Token: example.c:1:20: '^' "^",
	// · TypeQualifiers: []*cc.TypeQualifier{ // len 1
	// · · 0: &cc.TypeQualifier{
	// · · · Case: TypeQualifierNonnull,
	// · · · Token: example.c:1:22: '_Nonnull' "_Nonnull",
	// · · },
	// · },
	// }
}

func ExampleIndexExpr_index() {
	fmt.Println(exampleAST(17, "int i = x[y];"))
	// Output:
	// &cc.IndexExpr{
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · LeftBrace: example.c:1:10: '[' "[",
	// · Index: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "y",
	// · },
	// · RightBrace: example.c:1:12: ']' "]",
	// }
}

func ExampleCallExpr_call() {
	fmt.Println(exampleAST(18, "int i = x(y);"))
	// Output:
	// &cc.CallExpr{
	// · Func: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · LeftParen: example.c:1:10: '(' "(",
	// · Arguments: &cc.ArgumentExpressionList{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:11: identifier "y",
	// · · },
	// · },
	// · RightParen: example.c:1:12: ')' ")",
	// }
}

func ExampleSelectorExpr_select() {
	fmt.Println(exampleAST(19, "int i = x.y;"))
	// Output:
	// &cc.SelectorExpr{
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Token: example.c:1:10: '.' ".",
	// · Sel: example.c:1:11: identifier "y",
	// }
}

func ExampleSelectorExpr_pSelect() {
	fmt.Println(exampleAST(20, "int i = x->y;"))
	// Output:
	// &cc.SelectorExpr{
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Token: example.c:1:10: '->' "->",
	// · Sel: example.c:1:12: identifier "y",
	// · Ptr: true,
	// }
}

func ExamplePostfixExpr_inc() {
	fmt.Println(exampleAST(21, "int i = x++;"))
	// Output:
	// &cc.PostfixExpr{
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Token: example.c:1:10: '++' "++",
	// }
}

func ExamplePostfixExpr_dec() {
	fmt.Println(exampleAST(22, "int i = x--;"))
	// Output:
	// &cc.PostfixExpr{
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Token: example.c:1:10: '--' "--",
	// · Dec: true,
	// }
}

func ExampleCompositeLitExpr_complit() {
	fmt.Println(exampleAST(23, "int i = (int[]){y};"))
	// Output:
	// &cc.CompositeLitExpr{
	// · LeftParen: example.c:1:9: '(' "(",
	// · TypeName: &cc.TypeName{
	// · · AbstractDeclarator: &cc.AbstractDeclarator{
	// · · · Case: AbstractDeclaratorDecl,
	// · · · DirectAbstractDeclarator: &cc.DirectAbstractDeclarator{
	// · · · · Case: DirectAbstractDeclaratorArr,
	// · · · · Token: example.c:1:13: '[' "[",
	// · · · · Token2: example.c:1:14: ']' "]",
	// · · · },
	// · · },
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:10: 'int' "int",
	// · · · },
	// · · },
	// · },
	// · RightParen: example.c:1:15: ')' ")",
	// · LeftBrace: example.c:1:16: '{' "{",
	// · InitializerList: &cc.InitializerList{
	// · · Initializer: &cc.Initializer{
	// · · · Expression: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:17: identifier "y",
	// · · · },
	// · · · Case: InitializerExpr,
	// · · },
	// · },
	// · RightBrace: example.c:1:18: '}' "}",
	// }
}

func ExamplePrimaryExpression_ident() {
	fmt.Println(exampleAST(1, "int i = x;"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionIdent,
	// · Token: example.c:1:9: identifier "x",
	// }
}

func ExamplePrimaryExpression_int() {
	fmt.Println(exampleAST(2, "int i = 42;"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionInt,
	// · Token: example.c:1:9: integer constant "42",
	// }
}

func ExamplePrimaryExpression_float() {
	fmt.Println(exampleAST(3, "int i = 3.14;"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionFloat,
	// · Token: example.c:1:9: floating point constant "3.14",
	// }
}

func ExamplePrimaryExpression_char() {
	fmt.Println(exampleAST(4, "int i = 'x';"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionChar,
	// · Token: example.c:1:9: character constant "'x'",
	// }
}

func ExamplePrimaryExpression_lChar() {
	fmt.Println(exampleAST(5, "int i = L'x';"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionLChar,
	// · Token: example.c:1:9: long character constant "L'x'",
	// }
}

func ExamplePrimaryExpression_string() {
	fmt.Println(exampleAST(6, "char *c = \"x\" \"y\";"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionString,
	// · Token: example.c:1:11: string literal "\"xy\"",
	// }
}

func ExamplePrimaryExpression_lString() {
	fmt.Println(exampleAST(7, "char *c = L\"x\" L\"y\";"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionLString,
	// · Token: example.c:1:11: long string literal "L\"xy\"",
	// }
}

func ExamplePrimaryExpression_expr() {
	fmt.Println(exampleAST(8, "int i = (x+y);"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionExpr,
	// · ExpressionList: &cc.BinaryExpression{
	// · · Lhs: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:10: identifier "x",
	// · · },
	// · · Op: BinaryOperationAdd,
	// · · Token: example.c:1:11: '+' "+",
	// · · Rhs: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:12: identifier "y",
	// · · },
	// · },
	// · Token: example.c:1:9: '(' "(",
	// · Token2: example.c:1:13: ')' ")",
	// }
}

func ExamplePrimaryExpression_stmt() {
	fmt.Println(exampleAST(9, "int i = ({x();});"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionStmt,
	// · CompoundStatement: &cc.CompoundStatement{
	// · · Lbrace: example.c:1:10: '{' "{",
	// · · List: []*cc.ExpressionStatement{ // len 1
	// · · · 0: &cc.ExpressionStatement{
	// · · · · ExpressionList: &cc.CallExpr{
	// · · · · · Func: &cc.PrimaryExpression{
	// · · · · · · Case: PrimaryExpressionIdent,
	// · · · · · · Token: example.c:1:11: identifier "x",
	// · · · · · },
	// · · · · · LeftParen: example.c:1:12: '(' "(",
	// · · · · · RightParen: example.c:1:13: ')' ")",
	// · · · · },
	// · · · · Token: example.c:1:14: ';' ";",
	// · · · },
	// · · },
	// · · Rbrace: example.c:1:15: '}' "}",
	// · },
	// · Token: example.c:1:9: '(' "(",
	// · Token2: example.c:1:16: ')' ")",
	// }
}

func ExamplePrimaryExpression_generic() {
	fmt.Println(exampleAST(10, "int i = _Generic(x, int: y)(42);"))
	// Output:
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionGeneric,
	// · GenericSelection: &cc.GenericSelection{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:18: identifier "x",
	// · · },
	// · · GenericAssociationList: &cc.GenericAssociationList{
	// · · · GenericAssociation: &cc.GenericAssociation{
	// · · · · Expression: &cc.PrimaryExpression{
	// · · · · · Case: PrimaryExpressionIdent,
	// · · · · · Token: example.c:1:26: identifier "y",
	// · · · · },
	// · · · · Case: GenericAssociationType,
	// · · · · Token: example.c:1:24: ':' ":",
	// · · · · TypeName: &cc.TypeName{
	// · · · · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · · · · 0: &cc.TypeSpecName{
	// · · · · · · · Case: TypeSpecifierInt,
	// · · · · · · · Name: example.c:1:21: 'int' "int",
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · },
	// · · },
	// · · Token: example.c:1:9: '_Generic' "_Generic",
	// · · Token2: example.c:1:17: '(' "(",
	// · · Token3: example.c:1:19: ',' ",",
	// · · Token4: example.c:1:27: ')' ")",
	// · },
	// }
	// &cc.PrimaryExpression{
	// · Case: PrimaryExpressionInt,
	// · Token: example.c:1:29: integer constant "42",
	// }
}

func ExampleBinaryExpression_lt() {
	fmt.Println(exampleAST(55, "int i = x < y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationLt,
	// · Token: example.c:1:11: '<' "<",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:13: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_gt() {
	fmt.Println(exampleAST(56, "int i = x > y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationGt,
	// · Token: example.c:1:11: '>' ">",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:13: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_leq() {
	fmt.Println(exampleAST(57, "int i = x <= y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationLeq,
	// · Token: example.c:1:11: '<=' "<=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_geq() {
	fmt.Println(exampleAST(58, "int i = x >= y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationGeq,
	// · Token: example.c:1:11: '>=' ">=",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "y",
	// · },
	// }
}

func ExampleSelectionStatement_if() {
	fmt.Println(exampleAST(234, "int f() { if(x) y(); }"))
	// Output:
	// &cc.SelectionStatement{
	// · Case: SelectionStatementIf,
	// · ExpressionList: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "x",
	// · },
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:17: identifier "y",
	// · · · },
	// · · · LeftParen: example.c:1:18: '(' "(",
	// · · · RightParen: example.c:1:19: ')' ")",
	// · · },
	// · · Token: example.c:1:20: ';' ";",
	// · },
	// · Token: example.c:1:11: 'if' "if",
	// · Token2: example.c:1:13: '(' "(",
	// · Token3: example.c:1:15: ')' ")",
	// }
}

func ExampleSelectionStatement_ifElse() {
	fmt.Println(exampleAST(235, "int f() { if(x) y(); else z(); }"))
	// Output:
	// &cc.SelectionStatement{
	// · Case: SelectionStatementIfElse,
	// · ExpressionList: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "x",
	// · },
	// · Statement: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:17: identifier "y",
	// · · · },
	// · · · LeftParen: example.c:1:18: '(' "(",
	// · · · RightParen: example.c:1:19: ')' ")",
	// · · },
	// · · Token: example.c:1:20: ';' ";",
	// · },
	// · Statement2: &cc.ExpressionStatement{
	// · · ExpressionList: &cc.CallExpr{
	// · · · Func: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:27: identifier "z",
	// · · · },
	// · · · LeftParen: example.c:1:28: '(' "(",
	// · · · RightParen: example.c:1:29: ')' ")",
	// · · },
	// · · Token: example.c:1:30: ';' ";",
	// · },
	// · Token: example.c:1:11: 'if' "if",
	// · Token2: example.c:1:13: '(' "(",
	// · Token3: example.c:1:15: ')' ")",
	// · Token4: example.c:1:22: 'else' "else",
	// }
}

func ExampleSelectionStatement_switch() {
	fmt.Println(exampleAST(236, "int f() { switch(i) case 42: x(); }"))
	// Output:
	// &cc.SelectionStatement{
	// · Case: SelectionStatementSwitch,
	// · ExpressionList: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:18: identifier "i",
	// · },
	// · Statement: &cc.LabeledStatement{
	// · · Case: LabeledStatementCaseLabel,
	// · · Expression: &cc.ConstantExpression{
	// · · · Expression: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionInt,
	// · · · · Token: example.c:1:26: integer constant "42",
	// · · · },
	// · · },
	// · · Statement: &cc.ExpressionStatement{
	// · · · ExpressionList: &cc.CallExpr{
	// · · · · Func: &cc.PrimaryExpression{
	// · · · · · Case: PrimaryExpressionIdent,
	// · · · · · Token: example.c:1:30: identifier "x",
	// · · · · },
	// · · · · LeftParen: example.c:1:31: '(' "(",
	// · · · · RightParen: example.c:1:32: ')' ")",
	// · · · },
	// · · · Token: example.c:1:33: ';' ";",
	// · · },
	// · · Token: example.c:1:21: 'case' "case",
	// · · Token2: example.c:1:28: ':' ":",
	// · },
	// · Token: example.c:1:11: 'switch' "switch",
	// · Token2: example.c:1:17: '(' "(",
	// · Token3: example.c:1:19: ')' ")",
	// }
}

func ExampleBinaryExpression_lsh() {
	fmt.Println(exampleAST(52, "int i = x << y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationLsh,
	// · Token: example.c:1:11: '<<' "<<",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "y",
	// · },
	// }
}

func ExampleBinaryExpression_rsh() {
	fmt.Println(exampleAST(53, "int i = x >> y;"))
	// Output:
	// &cc.BinaryExpression{
	// · Lhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:9: identifier "x",
	// · },
	// · Op: BinaryOperationRsh,
	// · Token: example.c:1:11: '>>' ">>",
	// · Rhs: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:14: identifier "y",
	// · },
	// }
}

func ExampleTypeSpecName_sqTypeSpec() {
	fmt.Println(exampleAST(148, "struct {int i;};"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:9: 'int' "int",
	// }
}

func ExampleTypeQualifier_sqTypeQual() {
	fmt.Println(exampleAST(149, "struct {const int i;};"))
	// Output:
	// &cc.TypeQualifier{
	// · Case: TypeQualifierConst,
	// · Token: example.c:1:9: 'const' "const",
	// }
}

func ExampleAlignmentSpecifier_alignSpec() {
	fmt.Println(exampleAST(150, "struct {_Alignas(double) int i;};"))
	// Output:
	// &cc.AlignmentSpecifier{
	// · Case: AlignmentSpecifierType,
	// · Token: example.c:1:9: '_Alignas' "_Alignas",
	// · Token2: example.c:1:17: '(' "(",
	// · Token3: example.c:1:24: ')' ")",
	// · TypeName: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierDouble,
	// · · · · Name: example.c:1:18: 'double' "double",
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleStatement_labeled() {
	fmt.Println(exampleAST(214, "int f() { L: x(); }"))
	// Output:
	// &cc.CompoundStatement{
	// · Lbrace: example.c:1:9: '{' "{",
	// · List: []*cc.CommonDeclaration{ // len 2
	// · · 0: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · },
	// · · · · 1: &cc.TypeQualifier{
	// · · · · · Case: TypeQualifierConst,
	// · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · },
	// · · · · 2: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierChar,
	// · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · },
	// · · · · · },
	// · · · · · Initializer: &cc.Initializer{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · },
	// · · · · · · Case: InitializerExpr,
	// · · · · · },
	// · · · · · Token: example.c:1:9: '=' "=",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:9: ';' ";",
	// · · },
	// · · 1: &cc.LabeledStatement{
	// · · · Case: LabeledStatementLabel,
	// · · · Statement: &cc.ExpressionStatement{
	// · · · · ExpressionList: &cc.CallExpr{
	// · · · · · Func: &cc.PrimaryExpression{
	// · · · · · · Case: PrimaryExpressionIdent,
	// · · · · · · Token: example.c:1:14: identifier "x",
	// · · · · · },
	// · · · · · LeftParen: example.c:1:15: '(' "(",
	// · · · · · RightParen: example.c:1:16: ')' ")",
	// · · · · },
	// · · · · Token: example.c:1:17: ';' ";",
	// · · · },
	// · · · Token: example.c:1:11: identifier "L",
	// · · · Token2: example.c:1:12: ':' ":",
	// · · },
	// · },
	// · Rbrace: example.c:1:19: '}' "}",
	// }
}

func ExampleStatement_compound() {
	fmt.Println(exampleAST(215, "int f() { { y(); } }"))
	// Output:
	// &cc.CompoundStatement{
	// · Lbrace: example.c:1:9: '{' "{",
	// · List: []*cc.CommonDeclaration{ // len 2
	// · · 0: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · },
	// · · · · 1: &cc.TypeQualifier{
	// · · · · · Case: TypeQualifierConst,
	// · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · },
	// · · · · 2: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierChar,
	// · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · },
	// · · · · · },
	// · · · · · Initializer: &cc.Initializer{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · },
	// · · · · · · Case: InitializerExpr,
	// · · · · · },
	// · · · · · Token: example.c:1:9: '=' "=",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:9: ';' ";",
	// · · },
	// · · 1: &cc.CompoundStatement{
	// · · · Lbrace: example.c:1:11: '{' "{",
	// · · · List: []*cc.ExpressionStatement{ // len 1
	// · · · · 0: &cc.ExpressionStatement{
	// · · · · · ExpressionList: &cc.CallExpr{
	// · · · · · · Func: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionIdent,
	// · · · · · · · Token: example.c:1:13: identifier "y",
	// · · · · · · },
	// · · · · · · LeftParen: example.c:1:14: '(' "(",
	// · · · · · · RightParen: example.c:1:15: ')' ")",
	// · · · · · },
	// · · · · · Token: example.c:1:16: ';' ";",
	// · · · · },
	// · · · },
	// · · · Rbrace: example.c:1:18: '}' "}",
	// · · },
	// · },
	// · Rbrace: example.c:1:20: '}' "}",
	// }
}

func ExampleStatement_expr() {
	fmt.Println(exampleAST(216, "int f() { __attribute__((a)); }"))
	// Output:
	// &cc.CompoundStatement{
	// · Lbrace: example.c:1:9: '{' "{",
	// · List: []*cc.CommonDeclaration{ // len 2
	// · · 0: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · },
	// · · · · 1: &cc.TypeQualifier{
	// · · · · · Case: TypeQualifierConst,
	// · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · },
	// · · · · 2: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierChar,
	// · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · },
	// · · · · · },
	// · · · · · Initializer: &cc.Initializer{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · },
	// · · · · · · Case: InitializerExpr,
	// · · · · · },
	// · · · · · Token: example.c:1:9: '=' "=",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:9: ';' ";",
	// · · },
	// · · 1: &cc.ExpressionStatement{
	// · · · Token: example.c:1:29: ';' ";",
	// · · },
	// · },
	// · Rbrace: example.c:1:31: '}' "}",
	// }
}

func ExampleStatement_selection() {
	fmt.Println(exampleAST(217, "int f() { if(x) y(); }"))
	// Output:
	// &cc.CompoundStatement{
	// · Lbrace: example.c:1:9: '{' "{",
	// · List: []*cc.CommonDeclaration{ // len 2
	// · · 0: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · },
	// · · · · 1: &cc.TypeQualifier{
	// · · · · · Case: TypeQualifierConst,
	// · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · },
	// · · · · 2: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierChar,
	// · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · },
	// · · · · · },
	// · · · · · Initializer: &cc.Initializer{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · },
	// · · · · · · Case: InitializerExpr,
	// · · · · · },
	// · · · · · Token: example.c:1:9: '=' "=",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:9: ';' ";",
	// · · },
	// · · 1: &cc.SelectionStatement{
	// · · · Case: SelectionStatementIf,
	// · · · ExpressionList: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:14: identifier "x",
	// · · · },
	// · · · Statement: &cc.ExpressionStatement{
	// · · · · ExpressionList: &cc.CallExpr{
	// · · · · · Func: &cc.PrimaryExpression{
	// · · · · · · Case: PrimaryExpressionIdent,
	// · · · · · · Token: example.c:1:17: identifier "y",
	// · · · · · },
	// · · · · · LeftParen: example.c:1:18: '(' "(",
	// · · · · · RightParen: example.c:1:19: ')' ")",
	// · · · · },
	// · · · · Token: example.c:1:20: ';' ";",
	// · · · },
	// · · · Token: example.c:1:11: 'if' "if",
	// · · · Token2: example.c:1:13: '(' "(",
	// · · · Token3: example.c:1:15: ')' ")",
	// · · },
	// · },
	// · Rbrace: example.c:1:22: '}' "}",
	// }
}

func ExampleStatement_iteration() {
	fmt.Println(exampleAST(218, "int f() { for(;;) x(); }"))
	// Output:
	// &cc.CompoundStatement{
	// · Lbrace: example.c:1:9: '{' "{",
	// · List: []*cc.CommonDeclaration{ // len 2
	// · · 0: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · },
	// · · · · 1: &cc.TypeQualifier{
	// · · · · · Case: TypeQualifierConst,
	// · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · },
	// · · · · 2: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierChar,
	// · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · },
	// · · · · · },
	// · · · · · Initializer: &cc.Initializer{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · },
	// · · · · · · Case: InitializerExpr,
	// · · · · · },
	// · · · · · Token: example.c:1:9: '=' "=",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:9: ';' ";",
	// · · },
	// · · 1: &cc.IterationStatement{
	// · · · Case: IterationStatementFor,
	// · · · Statement: &cc.ExpressionStatement{
	// · · · · ExpressionList: &cc.CallExpr{
	// · · · · · Func: &cc.PrimaryExpression{
	// · · · · · · Case: PrimaryExpressionIdent,
	// · · · · · · Token: example.c:1:19: identifier "x",
	// · · · · · },
	// · · · · · LeftParen: example.c:1:20: '(' "(",
	// · · · · · RightParen: example.c:1:21: ')' ")",
	// · · · · },
	// · · · · Token: example.c:1:22: ';' ";",
	// · · · },
	// · · · Token: example.c:1:11: 'for' "for",
	// · · · Token2: example.c:1:14: '(' "(",
	// · · · Token3: example.c:1:15: ';' ";",
	// · · · Token4: example.c:1:16: ';' ";",
	// · · · Token5: example.c:1:17: ')' ")",
	// · · },
	// · },
	// · Rbrace: example.c:1:24: '}' "}",
	// }
}

func ExampleStatement_jump() {
	fmt.Println(exampleAST(219, "int f() { return x; }"))
	// Output:
	// &cc.CompoundStatement{
	// · Lbrace: example.c:1:9: '{' "{",
	// · List: []*cc.CommonDeclaration{ // len 2
	// · · 0: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · },
	// · · · · 1: &cc.TypeQualifier{
	// · · · · · Case: TypeQualifierConst,
	// · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · },
	// · · · · 2: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierChar,
	// · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · },
	// · · · · · },
	// · · · · · Initializer: &cc.Initializer{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · },
	// · · · · · · Case: InitializerExpr,
	// · · · · · },
	// · · · · · Token: example.c:1:9: '=' "=",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:9: ';' ";",
	// · · },
	// · · 1: &cc.JumpStatement{
	// · · · Case: JumpStatementReturn,
	// · · · ExpressionList: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:18: identifier "x",
	// · · · },
	// · · · Token: example.c:1:11: 'return' "return",
	// · · · Token2: example.c:1:19: ';' ";",
	// · · },
	// · },
	// · Rbrace: example.c:1:21: '}' "}",
	// }
}

func ExampleStatement_asm() {
	fmt.Println(exampleAST(220, "int f() { __asm__(\"nop\"); }"))
	// Output:
	// &cc.CompoundStatement{
	// · Lbrace: example.c:1:9: '{' "{",
	// · List: []*cc.CommonDeclaration{ // len 2
	// · · 0: &cc.CommonDeclaration{
	// · · · DeclarationSpecifiers: []*cc.StorageClassSpecifier{ // len 3
	// · · · · 0: &cc.StorageClassSpecifier{
	// · · · · · Case: StorageClassSpecifierStatic,
	// · · · · · Token: example.c:1:9: 'static' "static",
	// · · · · },
	// · · · · 1: &cc.TypeQualifier{
	// · · · · · Case: TypeQualifierConst,
	// · · · · · Token: example.c:1:9: 'const' "const",
	// · · · · },
	// · · · · 2: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierChar,
	// · · · · · Name: example.c:1:9: 'char' "char",
	// · · · · },
	// · · · },
	// · · · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · · · 0: &cc.InitDeclarator{
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorArr,
	// · · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · · Token: example.c:1:9: identifier "__func__",
	// · · · · · · · },
	// · · · · · · · Token: example.c:1:9: '[' "[",
	// · · · · · · · Token2: example.c:1:9: ']' "]",
	// · · · · · · },
	// · · · · · },
	// · · · · · Initializer: &cc.Initializer{
	// · · · · · · Expression: &cc.PrimaryExpression{
	// · · · · · · · Case: PrimaryExpressionString,
	// · · · · · · · Token: example.c:1:9: string literal "\"f\"",
	// · · · · · · },
	// · · · · · · Case: InitializerExpr,
	// · · · · · },
	// · · · · · Token: example.c:1:9: '=' "=",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:9: ';' ";",
	// · · },
	// · · 1: &cc.AsmStatement{
	// · · · Asm: &cc.Asm{
	// · · · · Token: example.c:1:11: '__asm__' "__asm__",
	// · · · · Token2: example.c:1:18: '(' "(",
	// · · · · Token3: example.c:1:19: string literal "\"nop\"",
	// · · · · Token4: example.c:1:24: ')' ")",
	// · · · },
	// · · · Token: example.c:1:25: ';' ";",
	// · · },
	// · },
	// · Rbrace: example.c:1:27: '}' "}",
	// }
}

func ExampleStaticAssertDeclaration_case0() {
	fmt.Println(exampleAST(92, "_Static_assert(x > y, \"abc\")"))
	// Output:
	// &cc.StaticAssertDeclaration{
	// · Expression: &cc.ConstantExpression{
	// · · Expression: &cc.BinaryExpression{
	// · · · Lhs: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:16: identifier "x",
	// · · · },
	// · · · Op: BinaryOperationGt,
	// · · · Token: example.c:1:18: '>' ">",
	// · · · Rhs: &cc.PrimaryExpression{
	// · · · · Case: PrimaryExpressionIdent,
	// · · · · Token: example.c:1:20: identifier "y",
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:1: _Static_assert "_Static_assert",
	// · Token2: example.c:1:15: '(' "(",
	// · Token3: example.c:1:21: ',' ",",
	// · Token4: example.c:1:23: string literal "\"abc\"",
	// · Token5: example.c:1:28: ')' ")",
	// }
}

func ExampleStorageClassSpecifier_typedef() {
	fmt.Println(exampleAST(103, "typedef int int_t;"))
	// Output:
	// &cc.StorageClassSpecifier{
	// · Case: StorageClassSpecifierTypedef,
	// · Token: example.c:1:1: 'typedef' "typedef",
	// }
}

func ExampleStorageClassSpecifier_extern() {
	fmt.Println(exampleAST(104, "extern int i;"))
	// Output:
	// &cc.StorageClassSpecifier{
	// · Case: StorageClassSpecifierExtern,
	// · Token: example.c:1:1: 'extern' "extern",
	// }
}

func ExampleStorageClassSpecifier_static() {
	fmt.Println(exampleAST(105, "static int i;"))
	// Output:
	// &cc.StorageClassSpecifier{
	// · Case: StorageClassSpecifierStatic,
	// · Token: example.c:1:1: 'static' "static",
	// }
}

func ExampleStorageClassSpecifier_auto() {
	fmt.Println(exampleAST(106, "auto int i;"))
	// Output:
	// &cc.StorageClassSpecifier{
	// · Case: StorageClassSpecifierAuto,
	// · Token: example.c:1:1: 'auto' "auto",
	// }
}

func ExampleStorageClassSpecifier_register() {
	fmt.Println(exampleAST(107, "register int i;"))
	// Output:
	// &cc.StorageClassSpecifier{
	// · Case: StorageClassSpecifierRegister,
	// · Token: example.c:1:1: 'register' "register",
	// }
}

func ExampleStorageClassSpecifier_threadLocal() {
	fmt.Println(exampleAST(108, "_Thread_local int i;"))
	// Output:
	// &cc.StorageClassSpecifier{
	// · Case: StorageClassSpecifierThreadLocal,
	// · Token: example.c:1:1: '_Thread_local' "_Thread_local",
	// }
}

func ExampleStorageClassSpecifier_declspec() {
	fmt.Println(exampleAST(109, "__declspec(foo) int i;"))
	// Output:
	// &cc.StorageClassSpecifier{
	// · Declspecs: []cc.Token{ // len 1
	// · · 0: example.c:1:12: identifier "foo",
	// · },
	// · Case: StorageClassSpecifierDeclspec,
	// · Token: example.c:1:1: '__declspec' "__declspec",
	// · Token2: example.c:1:11: '(' "(",
	// · Token3: example.c:1:15: ')' ")",
	// }
}

func ExampleStructDeclaration_decl() {
	fmt.Println(exampleAST(146, "struct{ int i __attribute__((a)); };"))
	// Output:
	// &cc.StructDeclaration{
	// · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · 0: &cc.AttributeSpecifier{
	// · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · Case: AttributeValueIdent,
	// · · · · · Token: example.c:1:30: identifier "a",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:15: '__attribute__' "__attribute__",
	// · · · Token2: example.c:1:28: '(' "(",
	// · · · Token3: example.c:1:29: '(' "(",
	// · · · Token4: example.c:1:31: ')' ")",
	// · · · Token5: example.c:1:32: ')' ")",
	// · · },
	// · },
	// · Case: StructDeclarationDecl,
	// · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:9: 'int' "int",
	// · · },
	// · },
	// · StructDeclaratorList: &cc.StructDeclaratorList{
	// · · StructDeclarator: &cc.StructDeclarator{
	// · · · Case: StructDeclaratorDecl,
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:13: identifier "i",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:33: ';' ";",
	// }
}

func ExampleStructDeclaration_assert() {
	fmt.Println(exampleAST(147, "struct{ _Static_assert(x > y, \"abc\"); };"))
	// Output:
	// &cc.StructDeclaration{
	// · Case: StructDeclarationAssert,
	// · StaticAssertDeclaration: &cc.StaticAssertDeclaration{
	// · · Expression: &cc.ConstantExpression{
	// · · · Expression: &cc.BinaryExpression{
	// · · · · Lhs: &cc.PrimaryExpression{
	// · · · · · Case: PrimaryExpressionIdent,
	// · · · · · Token: example.c:1:24: identifier "x",
	// · · · · },
	// · · · · Op: BinaryOperationGt,
	// · · · · Token: example.c:1:26: '>' ">",
	// · · · · Rhs: &cc.PrimaryExpression{
	// · · · · · Case: PrimaryExpressionIdent,
	// · · · · · Token: example.c:1:28: identifier "y",
	// · · · · },
	// · · · },
	// · · },
	// · · Token: example.c:1:9: _Static_assert "_Static_assert",
	// · · Token2: example.c:1:23: '(' "(",
	// · · Token3: example.c:1:29: ',' ",",
	// · · Token4: example.c:1:31: string literal "\"abc\"",
	// · · Token5: example.c:1:36: ')' ")",
	// · },
	// · Token: example.c:1:37: ';' ";",
	// }
}

func ExampleStructDeclarationList_case0() {
	fmt.Println(exampleAST(144, "struct{ __attribute__((a)) int i; };"))
	// Output:
	// &cc.StructDeclarationList{
	// · StructDeclaration: &cc.StructDeclaration{
	// · · Case: StructDeclarationDecl,
	// · · SpecifierQualifiers: []*cc.TypeQualifier{ // len 2
	// · · · 0: &cc.TypeQualifier{
	// · · · · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · · · · 0: &cc.AttributeSpecifier{
	// · · · · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · · · · Case: AttributeValueIdent,
	// · · · · · · · · Token: example.c:1:24: identifier "a",
	// · · · · · · · },
	// · · · · · · },
	// · · · · · · Token: example.c:1:9: '__attribute__' "__attribute__",
	// · · · · · · Token2: example.c:1:22: '(' "(",
	// · · · · · · Token3: example.c:1:23: '(' "(",
	// · · · · · · Token4: example.c:1:25: ')' ")",
	// · · · · · · Token5: example.c:1:26: ')' ")",
	// · · · · · },
	// · · · · },
	// · · · · Case: TypeQualifierAttr,
	// · · · },
	// · · · 1: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:28: 'int' "int",
	// · · · },
	// · · },
	// · · StructDeclaratorList: &cc.StructDeclaratorList{
	// · · · StructDeclarator: &cc.StructDeclarator{
	// · · · · Case: StructDeclaratorDecl,
	// · · · · Declarator: &cc.Declarator{
	// · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · Token: example.c:1:32: identifier "i",
	// · · · · · },
	// · · · · },
	// · · · },
	// · · },
	// · · Token: example.c:1:33: ';' ";",
	// · },
	// }
}

func ExampleStructDeclarationList_case1() {
	fmt.Println(exampleAST(145, "struct{ int i; double d; };"))
	// Output:
	// &cc.StructDeclarationList{
	// · StructDeclaration: &cc.StructDeclaration{
	// · · Case: StructDeclarationDecl,
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:9: 'int' "int",
	// · · · },
	// · · },
	// · · StructDeclaratorList: &cc.StructDeclaratorList{
	// · · · StructDeclarator: &cc.StructDeclarator{
	// · · · · Case: StructDeclaratorDecl,
	// · · · · Declarator: &cc.Declarator{
	// · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · Token: example.c:1:13: identifier "i",
	// · · · · · },
	// · · · · },
	// · · · },
	// · · },
	// · · Token: example.c:1:14: ';' ";",
	// · },
	// · StructDeclarationList: &cc.StructDeclarationList{
	// · · StructDeclaration: &cc.StructDeclaration{
	// · · · Case: StructDeclarationDecl,
	// · · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierDouble,
	// · · · · · Name: example.c:1:16: 'double' "double",
	// · · · · },
	// · · · },
	// · · · StructDeclaratorList: &cc.StructDeclaratorList{
	// · · · · StructDeclarator: &cc.StructDeclarator{
	// · · · · · Case: StructDeclaratorDecl,
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · Token: example.c:1:23: identifier "d",
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:24: ';' ";",
	// · · },
	// · },
	// }
}

func ExampleStructDeclarator_decl() {
	fmt.Println(exampleAST(153, "struct{ int i; };"))
	// Output:
	// &cc.StructDeclarator{
	// · Case: StructDeclaratorDecl,
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:13: identifier "i",
	// · · },
	// · },
	// }
}

func ExampleStructDeclarator_bitField() {
	fmt.Println(exampleAST(154, "struct{ int i:3; };"))
	// Output:
	// &cc.StructDeclarator{
	// · Case: StructDeclaratorBitField,
	// · Expression: &cc.ConstantExpression{
	// · · Expression: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionInt,
	// · · · Token: example.c:1:15: integer constant "3",
	// · · },
	// · },
	// · Declarator: &cc.Declarator{
	// · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · Case: DirectDeclaratorIdent,
	// · · · Token: example.c:1:13: identifier "i",
	// · · },
	// · },
	// · Token: example.c:1:14: ':' ":",
	// }
}

func ExampleStructDeclaratorList_case0() {
	fmt.Println(exampleAST(151, "struct{ int i; };"))
	// Output:
	// &cc.StructDeclaratorList{
	// · StructDeclarator: &cc.StructDeclarator{
	// · · Case: StructDeclaratorDecl,
	// · · Declarator: &cc.Declarator{
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:13: identifier "i",
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleStructDeclaratorList_case1() {
	fmt.Println(exampleAST(152, "struct{ int i, j; };"))
	// Output:
	// &cc.StructDeclaratorList{
	// · StructDeclarator: &cc.StructDeclarator{
	// · · Case: StructDeclaratorDecl,
	// · · Declarator: &cc.Declarator{
	// · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · Case: DirectDeclaratorIdent,
	// · · · · Token: example.c:1:13: identifier "i",
	// · · · },
	// · · },
	// · },
	// · StructDeclaratorList: &cc.StructDeclaratorList{
	// · · StructDeclarator: &cc.StructDeclarator{
	// · · · Case: StructDeclaratorDecl,
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:16: identifier "j",
	// · · · · },
	// · · · },
	// · · },
	// · · Token: example.c:1:14: ',' ",",
	// · },
	// }
}

func ExampleStructOrUnion_struct() {
	fmt.Println(exampleAST(142, "struct { int i; } s;"))
	// Output:
	// &cc.StructOrUnion{
	// · Case: StructOrUnionStruct,
	// · Token: example.c:1:1: 'struct' "struct",
	// }
}

func ExampleStructOrUnion_union() {
	fmt.Println(exampleAST(143, "union { int i; double d; } u;"))
	// Output:
	// &cc.StructOrUnion{
	// · Case: StructOrUnionUnion,
	// · Token: example.c:1:1: 'union' "union",
	// }
}

func ExampleStructOrUnionSpecifier_def() {
	fmt.Println(exampleAST(140, "struct s { int i; } __attribute__((a));"))
	// Output:
	// &cc.StructOrUnionSpecifier{
	// · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · 0: &cc.AttributeSpecifier{
	// · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · Case: AttributeValueIdent,
	// · · · · · Token: example.c:1:36: identifier "a",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:21: '__attribute__' "__attribute__",
	// · · · Token2: example.c:1:34: '(' "(",
	// · · · Token3: example.c:1:35: '(' "(",
	// · · · Token4: example.c:1:37: ')' ")",
	// · · · Token5: example.c:1:38: ')' ")",
	// · · },
	// · },
	// · Case: StructOrUnionSpecifierDef,
	// · StructDeclarationList: &cc.StructDeclarationList{
	// · · StructDeclaration: &cc.StructDeclaration{
	// · · · Case: StructDeclarationDecl,
	// · · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierInt,
	// · · · · · Name: example.c:1:12: 'int' "int",
	// · · · · },
	// · · · },
	// · · · StructDeclaratorList: &cc.StructDeclaratorList{
	// · · · · StructDeclarator: &cc.StructDeclarator{
	// · · · · · Case: StructDeclaratorDecl,
	// · · · · · Declarator: &cc.Declarator{
	// · · · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · · · Case: DirectDeclaratorIdent,
	// · · · · · · · Token: example.c:1:16: identifier "i",
	// · · · · · · },
	// · · · · · },
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:17: ';' ";",
	// · · },
	// · },
	// · StructOrUnion: &cc.StructOrUnion{
	// · · Case: StructOrUnionStruct,
	// · · Token: example.c:1:1: 'struct' "struct",
	// · },
	// · Token: example.c:1:8: identifier "s",
	// · Token2: example.c:1:10: '{' "{",
	// · Token3: example.c:1:19: '}' "}",
	// }
}

func ExampleStructOrUnionSpecifier_tag() {
	fmt.Println(exampleAST(141, "struct __attribute__((a)) s v;"))
	// Output:
	// &cc.StructOrUnionSpecifier{
	// · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · 0: &cc.AttributeSpecifier{
	// · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · Case: AttributeValueIdent,
	// · · · · · Token: example.c:1:23: identifier "a",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:8: '__attribute__' "__attribute__",
	// · · · Token2: example.c:1:21: '(' "(",
	// · · · Token3: example.c:1:22: '(' "(",
	// · · · Token4: example.c:1:24: ')' ")",
	// · · · Token5: example.c:1:25: ')' ")",
	// · · },
	// · },
	// · Case: StructOrUnionSpecifierTag,
	// · StructOrUnion: &cc.StructOrUnion{
	// · · Case: StructOrUnionStruct,
	// · · Token: example.c:1:1: 'struct' "struct",
	// · },
	// · Token: example.c:1:27: identifier "s",
	// }
}

func ExampleExternalDeclaration_case0() {
	ast := exampleASTRaw("int i;")
	fmt.Println(ast.Declarations[0])
	// Output:
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:5: identifier "i",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:6: ';' ";",
	// }
}

func ExampleExternalDeclaration_case1() {
	ast := exampleASTRaw("int i; int j;")
	fmt.Println(ast.Declarations[0])
	fmt.Println(ast.Declarations[1])
	// Output:
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:1: 'int' "int",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:5: identifier "i",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:6: ';' ";",
	// }
	// &cc.CommonDeclaration{
	// · DeclarationSpecifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:8: 'int' "int",
	// · · },
	// · },
	// · InitDeclarators: []*cc.InitDeclarator{ // len 1
	// · · 0: &cc.InitDeclarator{
	// · · · Declarator: &cc.Declarator{
	// · · · · DirectDeclarator: &cc.DirectDeclarator{
	// · · · · · Case: DirectDeclaratorIdent,
	// · · · · · Token: example.c:1:12: identifier "j",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// · Token: example.c:1:13: ';' ";",
	// }
}

func ExampleTypeName_case0() {
	fmt.Println(exampleAST(194, "int i = (int)x;"))
	// Output:
	// &cc.TypeName{
	// · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · 0: &cc.TypeSpecName{
	// · · · Case: TypeSpecifierInt,
	// · · · Name: example.c:1:10: 'int' "int",
	// · · },
	// · },
	// }
}

func ExampleTypeQualifier_const() {
	fmt.Println(exampleAST(162, "const int i;"))
	// Output:
	// &cc.TypeQualifier{
	// · Case: TypeQualifierConst,
	// · Token: example.c:1:1: 'const' "const",
	// }
}

func ExampleTypeQualifier_restrict() {
	fmt.Println(exampleAST(163, "restrict int i;"))
	// Output:
	// &cc.TypeQualifier{
	// · Case: TypeQualifierRestrict,
	// · Token: example.c:1:1: 'restrict' "restrict",
	// }
}

func ExampleTypeQualifier_volatile() {
	fmt.Println(exampleAST(164, "volatile int i;"))
	// Output:
	// &cc.TypeQualifier{
	// · Case: TypeQualifierVolatile,
	// · Token: example.c:1:1: 'volatile' "volatile",
	// }
}

func ExampleTypeQualifier_atomic() {
	fmt.Println(exampleAST(165, "_Atomic int i;"))
	// Output:
	// &cc.TypeQualifier{
	// · Case: TypeQualifierAtomic,
	// · Token: example.c:1:1: '_Atomic' "_Atomic",
	// }
}

func ExampleTypeQualifier_nonnull() {
	fmt.Println(exampleAST(166, "_Nonnull int i;"))
	// Output:
	// &cc.TypeQualifier{
	// · Case: TypeQualifierNonnull,
	// · Token: example.c:1:1: '_Nonnull' "_Nonnull",
	// }
}

func ExampleTypeQualifier_attr() {
	fmt.Println(exampleAST(167, "struct { __attribute__((a)) int i; };"))
	// Output:
	// &cc.TypeQualifier{
	// · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · 0: &cc.AttributeSpecifier{
	// · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · Case: AttributeValueIdent,
	// · · · · · Token: example.c:1:25: identifier "a",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:10: '__attribute__' "__attribute__",
	// · · · Token2: example.c:1:23: '(' "(",
	// · · · Token3: example.c:1:24: '(' "(",
	// · · · Token4: example.c:1:26: ')' ")",
	// · · · Token5: example.c:1:27: ')' ")",
	// · · },
	// · },
	// · Case: TypeQualifierAttr,
	// }
}

func ExampleTypeQualifier_typeQual() {
	fmt.Println(exampleAST(184, "int * __attribute__((a)) const i;"))
	// Output:
	// &cc.TypeQualifier{
	// · AttributeSpecifiers: []*cc.AttributeSpecifier{ // len 1
	// · · 0: &cc.AttributeSpecifier{
	// · · · AttributeValueList: &cc.AttributeValueList{
	// · · · · AttributeValue: &cc.AttributeValue{
	// · · · · · Case: AttributeValueIdent,
	// · · · · · Token: example.c:1:22: identifier "a",
	// · · · · },
	// · · · },
	// · · · Token: example.c:1:7: '__attribute__' "__attribute__",
	// · · · Token2: example.c:1:20: '(' "(",
	// · · · Token3: example.c:1:21: '(' "(",
	// · · · Token4: example.c:1:23: ')' ")",
	// · · · Token5: example.c:1:24: ')' ")",
	// · · },
	// · },
	// · Case: TypeQualifierAttr,
	// }
	// &cc.TypeQualifier{
	// · Case: TypeQualifierConst,
	// · Token: example.c:1:26: 'const' "const",
	// }
}

func ExampleTypeQualifier_case1() {
	fmt.Println(exampleAST(185, "int * const volatile i;"))
	// Output:
	// &cc.TypeQualifier{
	// · Case: TypeQualifierConst,
	// · Token: example.c:1:7: 'const' "const",
	// }
	// &cc.TypeQualifier{
	// · Case: TypeQualifierVolatile,
	// · Token: example.c:1:13: 'volatile' "volatile",
	// }
}

func ExampleTypeSpecName_void() {
	fmt.Println(exampleAST(110, "void i();"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierVoid,
	// · Name: example.c:1:1: 'void' "void",
	// }
}

func ExampleTypeSpecName_char() {
	fmt.Println(exampleAST(111, "char i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierChar,
	// · Name: example.c:1:1: 'char' "char",
	// }
}

func ExampleTypeSpecName_short() {
	fmt.Println(exampleAST(112, "short i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierShort,
	// · Name: example.c:1:1: 'short' "short",
	// }
}

func ExampleTypeSpecName_int() {
	fmt.Println(exampleAST(113, "int i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:1: 'int' "int",
	// }
}

func ExampleTypeSpecName_int128() {
	fmt.Println(exampleAST(114, "__int128 i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt128,
	// · Name: example.c:1:1: '__int128' "__int128",
	// }
}

func ExampleTypeSpecName_uint128() {
	fmt.Println(exampleAST(115, "__uint128_t i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierUint128,
	// · Name: example.c:1:1: '__uint128_t' "__uint128_t",
	// }
}

func ExampleTypeSpecName_long() {
	fmt.Println(exampleAST(116, "long i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierLong,
	// · Name: example.c:1:1: 'long' "long",
	// }
}

func ExampleTypeSpecName_float() {
	fmt.Println(exampleAST(117, "float i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierFloat,
	// · Name: example.c:1:1: 'float' "float",
	// }
}

func ExampleTypeSpecName_float16() {
	fmt.Println(exampleAST(118, "_Float16 i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierFloat16,
	// · Name: example.c:1:1: '_Float16' "_Float16",
	// }
}

func ExampleTypeSpecName_decimal32() {
	fmt.Println(exampleAST(119, "_Decimal32 i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierDecimal32,
	// · Name: example.c:1:1: '_Decimal32' "_Decimal32",
	// }
}

func ExampleTypeSpecName_decimal64() {
	fmt.Println(exampleAST(120, "_Decimal64 i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierDecimal64,
	// · Name: example.c:1:1: '_Decimal64' "_Decimal64",
	// }
}

func ExampleTypeSpecName_decimal128() {
	fmt.Println(exampleAST(121, "_Decimal128 i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierDecimal128,
	// · Name: example.c:1:1: '_Decimal128' "_Decimal128",
	// }
}

func ExampleTypeSpecName_float128() {
	fmt.Println(exampleAST(122, "_Float128 i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierFloat128,
	// · Name: example.c:1:1: '_Float128' "_Float128",
	// }
}

func ExampleTypeSpecName_float128x() {
	fmt.Println(exampleAST(123, "_Float128x i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierFloat128x,
	// · Name: example.c:1:1: '_Float128x' "_Float128x",
	// }
}

func ExampleTypeSpecName_double() {
	fmt.Println(exampleAST(124, "double i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierDouble,
	// · Name: example.c:1:1: 'double' "double",
	// }
}

func ExampleTypeSpecName_signed() {
	fmt.Println(exampleAST(125, "signed i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierSigned,
	// · Name: example.c:1:1: 'signed' "signed",
	// }
}

func ExampleTypeSpecName_unsigned() {
	fmt.Println(exampleAST(126, "unsigned i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierUnsigned,
	// · Name: example.c:1:1: 'unsigned' "unsigned",
	// }
}

func ExampleTypeSpecName_bool() {
	fmt.Println(exampleAST(127, "_Bool i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierBool,
	// · Name: example.c:1:1: '_Bool' "_Bool",
	// }
}

func ExampleTypeSpecName_complex() {
	fmt.Println(exampleAST(128, "_Complex i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierComplex,
	// · Name: example.c:1:1: '_Complex' "_Complex",
	// }
}

func ExampleTypeSpecName_imaginary() {
	fmt.Println(exampleAST(129, "_Imaginary i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierImaginary,
	// · Name: example.c:1:1: '_Imaginary' "_Imaginary",
	// }
}

func ExampleTypeSpecStructOrUnion_structOrUnion() {
	fmt.Println(exampleAST(130, "struct s i;"))
	// Output:
	// &cc.TypeSpecStructOrUnion{
	// · StructOrUnion: &cc.StructOrUnionSpecifier{
	// · · Case: StructOrUnionSpecifierTag,
	// · · StructOrUnion: &cc.StructOrUnion{
	// · · · Case: StructOrUnionStruct,
	// · · · Token: example.c:1:1: 'struct' "struct",
	// · · },
	// · · Token: example.c:1:8: identifier "s",
	// · },
	// }
}

func ExampleTypeSpecEnum_enum() {
	fmt.Println(exampleAST(131, "enum e i;"))
	// Output:
	// &cc.TypeSpecEnum{
	// · Enum: &cc.EnumSpecifier{
	// · · Case: EnumSpecifierTag,
	// · · Token: example.c:1:1: 'enum' "enum",
	// · · Token2: example.c:1:6: identifier "e",
	// · },
	// }
}

func ExampleTypeSpecName_typeName() {
	fmt.Println(exampleAST(132, "typedef int T; T i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierInt,
	// · Name: example.c:1:9: 'int' "int",
	// }
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierTypeName,
	// · Name: example.c:1:16: type name "T",
	// }
}

func ExampleTypeSpecTypeOfExpr_typeofExpr() {
	fmt.Println(exampleAST(133, "typeof(42) i;"))
	// Output:
	// &cc.TypeSpecTypeOfExpr{
	// · TypeOf: example.c:1:1: 'typeof' "typeof",
	// · LeftParen: example.c:1:7: '(' "(",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionInt,
	// · · Token: example.c:1:8: integer constant "42",
	// · },
	// · RightParen: example.c:1:10: ')' ")",
	// }
}

func ExampleTypeSpecTypeOfType_typeofType() {
	fmt.Println(exampleAST(134, "typedef int T; typeof(T) i;"))
	// Output:
	// &cc.TypeSpecTypeOfType{
	// · TypeOf: example.c:1:16: 'typeof' "typeof",
	// · LeftParen: example.c:1:22: '(' "(",
	// · Name: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierTypeName,
	// · · · · Name: example.c:1:23: type name "T",
	// · · · },
	// · · },
	// · },
	// · RightParen: example.c:1:24: ')' ")",
	// }
}

func ExampleTypeSpecAtomic_atomic() {
	fmt.Println(exampleAST(135, "_Atomic(int) i;"))
	// Output:
	// &cc.TypeSpecAtomic{
	// · Atomic: &cc.AtomicTypeSpecifier{
	// · · Token: example.c:1:1: '_Atomic' "_Atomic",
	// · · Token2: example.c:1:8: '(' "(",
	// · · Token3: example.c:1:12: ')' ")",
	// · · TypeName: &cc.TypeName{
	// · · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · · 0: &cc.TypeSpecName{
	// · · · · · Case: TypeSpecifierInt,
	// · · · · · Name: example.c:1:9: 'int' "int",
	// · · · · },
	// · · · },
	// · · },
	// · },
	// }
}

func ExampleTypeSpecName_float32() {
	fmt.Println(exampleAST(136, "_Float32 i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierFloat32,
	// · Name: example.c:1:1: '_Float32' "_Float32",
	// }
}

func ExampleTypeSpecName_float64() {
	fmt.Println(exampleAST(137, "_Float64 i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierFloat64,
	// · Name: example.c:1:1: '_Float64' "_Float64",
	// }
}

func ExampleTypeSpecName_float32x() {
	fmt.Println(exampleAST(138, "_Float32x i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierFloat32x,
	// · Name: example.c:1:1: '_Float32x' "_Float32x",
	// }
}

func ExampleTypeSpecName_float64x() {
	fmt.Println(exampleAST(139, "_Float64x i;"))
	// Output:
	// &cc.TypeSpecName{
	// · Case: TypeSpecifierFloat64x,
	// · Name: example.c:1:1: '_Float64x' "_Float64x",
	// }
}

func ExamplePrefixExpr_inc() {
	fmt.Println(exampleAST(27, "int i = ++x;"))
	// Output:
	// &cc.PrefixExpr{
	// · Token: example.c:1:9: '++' "++",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// }
}

func ExamplePrefixExpr_dec() {
	fmt.Println(exampleAST(28, "int i = --x;"))
	// Output:
	// &cc.PrefixExpr{
	// · Token: example.c:1:9: '--' "--",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// · Dec: true,
	// }
}

func ExampleUnaryExpr_addrof() {
	fmt.Println(exampleAST(29, "int *i = &x;"))
	// Output:
	// &cc.UnaryExpr{
	// · Case: UnaryExpressionAddrof,
	// · Token: example.c:1:10: '&' "&",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:11: identifier "x",
	// · },
	// }
}

func ExampleUnaryExpr_deref() {
	fmt.Println(exampleAST(30, "int i = *x;"))
	// Output:
	// &cc.UnaryExpr{
	// · Case: UnaryExpressionDeref,
	// · Token: example.c:1:9: '*' "*",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:10: identifier "x",
	// · },
	// }
}

func ExampleUnaryExpr_plus() {
	fmt.Println(exampleAST(31, "int i = +x;"))
	// Output:
	// &cc.UnaryExpr{
	// · Case: UnaryExpressionPlus,
	// · Token: example.c:1:9: '+' "+",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:10: identifier "x",
	// · },
	// }
}

func ExampleUnaryExpr_minus() {
	fmt.Println(exampleAST(32, "int i = -x;"))
	// Output:
	// &cc.UnaryExpr{
	// · Case: UnaryExpressionMinus,
	// · Token: example.c:1:9: '-' "-",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:10: identifier "x",
	// · },
	// }
}

func ExampleUnaryExpr_cpl() {
	fmt.Println(exampleAST(33, "int i = ~x;"))
	// Output:
	// &cc.UnaryExpr{
	// · Case: UnaryExpressionCpl,
	// · Token: example.c:1:9: '~' "~",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:10: identifier "x",
	// · },
	// }
}

func ExampleUnaryExpr_not() {
	fmt.Println(exampleAST(34, "int i = !x;"))
	// Output:
	// &cc.UnaryExpr{
	// · Case: UnaryExpressionNot,
	// · Token: example.c:1:9: '!' "!",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:10: identifier "x",
	// · },
	// }
}

func ExampleSizeOfExpr_sizeofExpr() {
	fmt.Println(exampleAST(35, "int i = sizeof x;"))
	// Output:
	// &cc.SizeOfExpr{
	// · Token: example.c:1:9: 'sizeof' "sizeof",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:16: identifier "x",
	// · },
	// }
}

func ExampleSizeOfTypeExpr_sizeofType() {
	fmt.Println(exampleAST(36, "int i = sizeof(int);"))
	// Output:
	// &cc.SizeOfTypeExpr{
	// · Token: example.c:1:9: 'sizeof' "sizeof",
	// · LeftParen: example.c:1:15: '(' "(",
	// · TypeName: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:16: 'int' "int",
	// · · · },
	// · · },
	// · },
	// · RightParen: example.c:1:19: ')' ")",
	// }
}

func ExampleLabelAddrExpr_labelAddr() {
	fmt.Println(exampleAST(37, "int f() { L: &&L; }"))
	// Output:
	// &cc.LabelAddrExpr{
	// · Token: example.c:1:14: '&&' "&&",
	// · Label: example.c:1:16: identifier "L",
	// }
}

func ExampleAlignOfExpr_alignofExpr() {
	fmt.Println(exampleAST(38, "int i = _Alignof(x);"))
	// Output:
	// &cc.AlignOfExpr{
	// · Token: example.c:1:9: '_Alignof' "_Alignof",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionExpr,
	// · · ExpressionList: &cc.PrimaryExpression{
	// · · · Case: PrimaryExpressionIdent,
	// · · · Token: example.c:1:18: identifier "x",
	// · · },
	// · · Token: example.c:1:17: '(' "(",
	// · · Token2: example.c:1:19: ')' ")",
	// · },
	// }
}

func ExampleAlignOfTypeExpr_alignofType() {
	fmt.Println(exampleAST(39, "int i = _Alignof(int);"))
	// Output:
	// &cc.AlignOfTypeExpr{
	// · Token: example.c:1:9: '_Alignof' "_Alignof",
	// · LeftParen: example.c:1:17: '(' "(",
	// · TypeName: &cc.TypeName{
	// · · SpecifierQualifiers: []*cc.TypeSpecName{ // len 1
	// · · · 0: &cc.TypeSpecName{
	// · · · · Case: TypeSpecifierInt,
	// · · · · Name: example.c:1:18: 'int' "int",
	// · · · },
	// · · },
	// · },
	// · RightParen: example.c:1:21: ')' ")",
	// }
}

func ExampleUnaryExpr_imag() {
	fmt.Println(exampleAST(40, "double i = __imag__ x;"))
	// Output:
	// &cc.UnaryExpr{
	// · Case: UnaryExpressionImag,
	// · Token: example.c:1:12: '__imag__' "__imag__",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:21: identifier "x",
	// · },
	// }
}

func ExampleUnaryExpr_real() {
	fmt.Println(exampleAST(41, "double i = __real__ x;"))
	// Output:
	// &cc.UnaryExpr{
	// · Case: UnaryExpressionReal,
	// · Token: example.c:1:12: '__real__' "__real__",
	// · Expr: &cc.PrimaryExpression{
	// · · Case: PrimaryExpressionIdent,
	// · · Token: example.c:1:21: identifier "x",
	// · },
	// }
}
