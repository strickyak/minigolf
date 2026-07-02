#include <stdarg.h>

typedef unsigned int intptr_t;

#define union struct // will this work?

#define HEAP_SIZE (100 << 10)  // 100KB

//# 1 "c-demos/gitlab.com-zsaleeba-picoc/main.c"
//# 1 "<built-in>"
//# 1 "<command-line>"
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/main.c"
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/clibrary.c" 1



//# 1 "c-demos/gitlab.com-zsaleeba-picoc/picoc.h" 1
//# 19 "c-demos/gitlab.com-zsaleeba-picoc/picoc.h"
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/interpreter.h" 1







//# 1 "c-demos/gitlab.com-zsaleeba-picoc/platform.h" 1
//# 92 "c-demos/gitlab.com-zsaleeba-picoc/platform.h"
//# 1 "golflib/stdio.h" 1


typedef struct stdio_FILE { int fd; } FILE;

FILE *stdin;
FILE *stdout;
FILE *stderr;
//# 93 "c-demos/gitlab.com-zsaleeba-picoc/platform.h" 2
//# 1 "golflib/assert.h" 1



void abort(void);

void assert(int pred) {
    if (!pred) {
        const char* s = "\n*** golflib: ASSERT FAILED\n";
        while (*s) {
            putchar(*s);
            s++;
        }
        abort();
    }
}
//# 94 "c-demos/gitlab.com-zsaleeba-picoc/platform.h" 2
//# 160 "c-demos/gitlab.com-zsaleeba-picoc/platform.h"
extern int ExitBuf[];
//# 9 "c-demos/gitlab.com-zsaleeba-picoc/interpreter.h" 2
//# 35 "c-demos/gitlab.com-zsaleeba-picoc/interpreter.h"
typedef struct OutputStream IOFILE;
//# 58 "c-demos/gitlab.com-zsaleeba-picoc/interpreter.h"
struct Table;
struct Picoc_Struct;

// typedef struct Picoc_Struct Picoc;


enum LexToken
{
               TokenNone,
               TokenComma,
               TokenAssign, TokenAddAssign, TokenSubtractAssign, TokenMultiplyAssign, TokenDivideAssign, TokenModulusAssign,
               TokenShiftLeftAssign, TokenShiftRightAssign, TokenArithmeticAndAssign, TokenArithmeticOrAssign, TokenArithmeticExorAssign,
               TokenQuestionMark, TokenColon,
               TokenLogicalOr,
               TokenLogicalAnd,
               TokenArithmeticOr,
               TokenArithmeticExor,
               TokenAmpersand,
               TokenEqual, TokenNotEqual,
               TokenLessThan, TokenGreaterThan, TokenLessEqual, TokenGreaterEqual,
               TokenShiftLeft, TokenShiftRight,
               TokenPlus, TokenMinus,
               TokenAsterisk, TokenSlash, TokenModulus,
               TokenIncrement, TokenDecrement, TokenUnaryNot, TokenUnaryExor, TokenSizeof, TokenCast,
               TokenLeftSquareBracket, TokenRightSquareBracket, TokenDot, TokenArrow,
               TokenOpenBracket, TokenCloseBracket,
               TokenIdentifier, TokenIntegerConstant,
               TokenFPConstant,
               TokenStringConstant, TokenCharacterConstant,
               TokenSemicolon, TokenEllipsis,
               TokenLeftBrace, TokenRightBrace,
               TokenIntType, TokenCharType, TokenFloatType, TokenDoubleType, TokenVoidType, TokenEnumType,
               TokenLongType, TokenSignedType, TokenShortType, TokenStaticType, TokenAutoType, TokenRegisterType, TokenExternType, TokenStructType, TokenUnionType, TokenUnsignedType, TokenTypedef,
               TokenContinue, TokenDo, TokenElse, TokenFor, TokenGoto, TokenIf, TokenWhile, TokenBreak, TokenSwitch, TokenCase, TokenDefault, TokenReturn,
               TokenHashDefine, TokenHashInclude, TokenHashIf, TokenHashIfdef, TokenHashIfndef, TokenHashElse, TokenHashEndif,
               TokenNew, TokenDelete,
               TokenOpenMacroBracket,
               TokenEOF, TokenEndOfLine, TokenEndOfFunction
};


struct AllocNode
{
    unsigned int Size;
    struct AllocNode *NextFree;
};


enum RunMode
{
    RunModeRun,
    RunModeSkip,
    RunModeReturn,
    RunModeCaseSearch,
    RunModeBreak,
    RunModeContinue,
    RunModeGoto
};


struct ParseState
{
    struct Picoc_Struct *pc;
    const unsigned char *Pos;
    char *FileName;
    short int Line;
    short int CharacterPos;
    enum RunMode Mode;
    int SearchLabel;
    const char *SearchGotoLabel;
    const char *SourceText;
    short int HashIfLevel;
    short int HashIfEvaluateToLevel;
    char DebugMode;
    int ScopeID;
};


enum BaseType
{
    TypeVoid,
    TypeInt,
    TypeShort,
    TypeChar,
    TypeLong,
    TypeUnsignedInt,
    TypeUnsignedShort,
    TypeUnsignedChar,
    TypeUnsignedLong,

    TypeFP,

    TypeFunction,
    TypeMacro,
    TypePointer,
    TypeArray,
    TypeStruct,
    TypeUnion,
    TypeEnum,
    TypeGotoLabel,
    Type_Type
};


struct ValueType
{
    enum BaseType Base;
    int ArraySize;
    int Sizeof;
    int AlignBytes;
    const char *Identifier;
    struct ValueType *FromType;
    struct ValueType *DerivedTypeList;
    struct ValueType *Next;
    struct Table *Members;
    int OnHeap;
    int StaticQualifier;
};


struct FuncDef
{
    struct ValueType *ReturnType;
    int NumParams;
    int VarArgs;
    struct ValueType **ParamType;
    char **ParamName;
    void (*Intrinsic)();
    struct ParseState Body;
};


struct MacroDef
{
    int NumParams;
    char **ParamName;
    struct ParseState Body;
};


union AnyValue
{
    char Character;
    short ShortInteger;
    int Integer;
    long LongInteger;
    unsigned short UnsignedShortInteger;
    unsigned int UnsignedInteger;
    unsigned long UnsignedLongInteger;
    unsigned char UnsignedCharacter;
    char *Identifier;
    char ArrayMem[2];
    struct ValueType *Typ;
    struct FuncDef FuncDef;
    struct MacroDef MacroDef;

    // double FP;

    void *Pointer;
};

struct Value
{
    struct ValueType *Typ;
    union AnyValue *Val;
    struct Value *LValueFrom;
    char ValOnHeap;
    char ValOnStack;
    char AnyValOnHeap;
    char IsLValue;
    int ScopeID;
    char OutOfScope;
};

struct ValueEntry
{
    char *Key;
    struct Value *Val;
};
struct BreakpointEntry
{
    const char *FileName;
    short int Line;
    short int CharacterPos;
};
union TableEntryPayload
{
        struct ValueEntry v;

        char Key[1];

        struct BreakpointEntry b;

};

struct TableEntry
{
    struct TableEntry *Next;
    const char *DeclFileName;
    unsigned short DeclLine;
    unsigned short DeclColumn;

    union TableEntryPayload p;
};

struct Table
{
    short Size;
    short OnHeap;
    struct TableEntry **HashTable;
};


struct StackFrame
{
    struct ParseState ReturnParser;
    const char *FuncName;
    struct Value *ReturnValue;
    struct Value **Parameter;
    int NumParams;
    struct Table LocalTable;
    struct TableEntry *LocalHashTable[11];
    struct StackFrame *PreviousStackFrame;
};


enum LexMode
{
    LexModeNormal,
    LexModeHashInclude,
    LexModeHashDefine,
    LexModeHashDefineSpace,
    LexModeHashDefineSpaceIdent
};

struct LexState
{
    const char *Pos;
    const char *End;
    const char *FileName;
    int Line;
    int CharacterPos;
    const char *SourceText;
    enum LexMode Mode;
    int EmitExtraNewlines;
};


struct LibraryFunction
{
    void (*Func)(struct ParseState *Parser, struct Value *, struct Value **, int);
    const char *Prototype;
};


struct StringOutputStream
    {
        struct ParseState *Parser;
        char *WritePos;
};
union OutputStreamInfo
{
    struct StringOutputStream Str;
};


typedef void CharWriter(unsigned char, union OutputStreamInfo *);


struct OutputStream
{
    CharWriter *Putch;
    union OutputStreamInfo i;
};


enum ParseResult { ParseResultEOF, ParseResultError, ParseResultOk };


struct CleanupTokenNode
{
    void *Tokens;
    const char *SourceText;
    struct CleanupTokenNode *Next;
};


struct TokenLine
{
    struct TokenLine *Next;
    unsigned char *Tokens;
    int NumBytes;
};



struct IncludeLibrary
{
    char *IncludeName;
    void (*SetupFunction)(struct Picoc_Struct *pc);
    struct LibraryFunction *FuncList;
    const char *SetupCSource;
    struct IncludeLibrary *NextLib;
};







struct Picoc_Struct
{

    struct Table GlobalTable;
    struct CleanupTokenNode *CleanupTokenList;
    struct TableEntry *GlobalHashTable[97];


    struct TokenLine *InteractiveHead;
    struct TokenLine *InteractiveTail;
    struct TokenLine *InteractiveCurrentLine;
    int LexUseStatementPrompt;
    union AnyValue LexAnyValue;
    struct Value LexValue;
    struct Table ReservedWordTable;
    struct TableEntry *ReservedWordHashTable[97];


    struct Table StringLiteralTable;
    struct TableEntry *StringLiteralHashTable[97];


    struct StackFrame *TopStackFrame;


    int PicocExitValue;


    struct IncludeLibrary *IncludeLibList;
//# 407 "c-demos/gitlab.com-zsaleeba-picoc/interpreter.h"
    unsigned char HeapMemory[(16*1024)];
    void *HeapBottom;
    void *StackFrame;
    void *HeapStackTop;



    struct AllocNode *FreeListBucket[8];
    struct AllocNode *FreeListBig;


    struct ValueType UberType;
    struct ValueType IntType;
    struct ValueType ShortType;
    struct ValueType CharType;
    struct ValueType LongType;
    struct ValueType UnsignedIntType;
    struct ValueType UnsignedShortType;
    struct ValueType UnsignedLongType;
    struct ValueType UnsignedCharType;

    //struct ValueType FPType;

    struct ValueType VoidType;
    struct ValueType TypeType;
    struct ValueType FunctionType;
    struct ValueType MacroType;
    struct ValueType EnumType;
    struct ValueType GotoLabelType;
    struct ValueType *CharPtrType;
    struct ValueType *CharPtrPtrType;
    struct ValueType *CharArrayType;
    struct ValueType *VoidPtrType;


    struct Table BreakpointTable;
    struct TableEntry *BreakpointHashTable[21];
    int BreakpointCount;
    int DebugManualBreak;


    int BigEndian;
    int LittleEndian;

    IOFILE *CStdOut;
    IOFILE CStdOutBase;


    const char *VersionString;
//# 466 "c-demos/gitlab.com-zsaleeba-picoc/interpreter.h"
    struct Table StringTable;
    struct TableEntry *StringHashTable[97];
    char *StrEmpty;
};

#define Picoc struct Picoc_Struct


void TableInit(Picoc *pc);
char *TableStrRegister(Picoc *pc, const char *Str);
char *TableStrRegister2(Picoc *pc, const char *Str, int Len);
void TableInitTable(struct Table *Tbl, struct TableEntry **HashTable, int Size, int OnHeap);
int TableSet(Picoc *pc, struct Table *Tbl, char *Key, struct Value *Val, const char *DeclFileName, int DeclLine, int DeclColumn);
int TableGet(struct Table *Tbl, const char *Key, struct Value **Val, const char **DeclFileName, int *DeclLine, int *DeclColumn);
struct Value *TableDelete(Picoc *pc, struct Table *Tbl, const char *Key);
char *TableSetIdentifier(Picoc *pc, struct Table *Tbl, const char *Ident, int IdentLen);
void TableStrFree(Picoc *pc);


void LexInit(Picoc *pc);
void LexCleanup(Picoc *pc);
void *LexAnalyse(Picoc *pc, const char *FileName, const char *Source, int SourceLen, int *TokenLen);
void LexInitParser(struct ParseState *Parser, Picoc *pc, const char *SourceText, void *TokenSource, char *FileName, int RunIt, int SetDebugMode);
enum LexToken LexGetToken(struct ParseState *Parser, struct Value **Value, int IncPos);
enum LexToken LexRawPeekToken(struct ParseState *Parser);
void LexToEndOfLine(struct ParseState *Parser);
void *LexCopyTokens(struct ParseState *StartParser, struct ParseState *EndParser);
void LexInteractiveClear(Picoc *pc, struct ParseState *Parser);
void LexInteractiveCompleted(Picoc *pc, struct ParseState *Parser);
void LexInteractiveStatementPrompt(Picoc *pc);





void PicocParseInteractiveNoStartPrompt(Picoc *pc, int EnableDebugger);
enum ParseResult ParseStatement(struct ParseState *Parser, int CheckTrailingSemicolon);
struct Value *ParseFunctionDefinition(struct ParseState *Parser, struct ValueType *ReturnType, char *Identifier);
void ParseCleanup(Picoc *pc);
void ParserCopyPos(struct ParseState *To, struct ParseState *From);
void ParserCopy(struct ParseState *To, struct ParseState *From);


int ExpressionParse(struct ParseState *Parser, struct Value **Result);
long ExpressionParseInt(struct ParseState *Parser);
void ExpressionAssign(struct ParseState *Parser, struct Value *DestValue, struct Value *SourceValue, int Force, const char *FuncName, int ParamNo, int AllowPointerCoercion);
long ExpressionCoerceInteger(struct Value *Val);
unsigned long ExpressionCoerceUnsignedInteger(struct Value *Val);

//double ExpressionCoerceFP(struct Value *Val);



void TypeInit(Picoc *pc);
void TypeCleanup(Picoc *pc);
int TypeSize(struct ValueType *Typ, int ArraySize, int Compact);
int TypeSizeValue(struct Value *Val, int Compact);
int TypeStackSizeValue(struct Value *Val);
int TypeLastAccessibleOffset(Picoc *pc, struct Value *Val);
int TypeParseFront(struct ParseState *Parser, struct ValueType **Typ, int *IsStatic);
void TypeParseIdentPart(struct ParseState *Parser, struct ValueType *BasicTyp, struct ValueType **Typ, char **Identifier);
void TypeParse(struct ParseState *Parser, struct ValueType **Typ, char **Identifier, int *IsStatic);
struct ValueType *TypeGetMatching(Picoc *pc, struct ParseState *Parser, struct ValueType *ParentType, enum BaseType Base, int ArraySize, const char *Identifier, int AllowDuplicates);
struct ValueType *TypeCreateOpaqueStruct(Picoc *pc, struct ParseState *Parser, const char *StructName, int Size);
int TypeIsForwardDeclared(struct ParseState *Parser, struct ValueType *Typ);


void HeapInit(Picoc *pc, int StackSize);
void HeapCleanup(Picoc *pc);
void *HeapAllocStack(Picoc *pc, int Size);
int HeapPopStack(Picoc *pc, void *Addr, int Size);
void HeapUnpopStack(Picoc *pc, int Size);
void HeapPushStackFrame(Picoc *pc);
int HeapPopStackFrame(Picoc *pc);
void *HeapAllocMem(Picoc *pc, int Size);
void HeapFreeMem(Picoc *pc, void *Mem);


void VariableInit(Picoc *pc);
void VariableCleanup(Picoc *pc);
void VariableFree(Picoc *pc, struct Value *Val);
void VariableTableCleanup(Picoc *pc, struct Table *HashTable);
void *VariableAlloc(Picoc *pc, struct ParseState *Parser, int Size, int OnHeap);
void VariableStackPop(struct ParseState *Parser, struct Value *Var);
struct Value *VariableAllocValueAndData(Picoc *pc, struct ParseState *Parser, int DataSize, int IsLValue, struct Value *LValueFrom, int OnHeap);
struct Value *VariableAllocValueAndCopy(Picoc *pc, struct ParseState *Parser, struct Value *FromValue, int OnHeap);
struct Value *VariableAllocValueFromType(Picoc *pc, struct ParseState *Parser, struct ValueType *Typ, int IsLValue, struct Value *LValueFrom, int OnHeap);
struct Value *VariableAllocValueFromExistingData(struct ParseState *Parser, struct ValueType *Typ, union AnyValue *FromValue, int IsLValue, struct Value *LValueFrom);
struct Value *VariableAllocValueShared(struct ParseState *Parser, struct Value *FromValue);
struct Value *VariableDefine(Picoc *pc, struct ParseState *Parser, char *Ident, struct Value *InitValue, struct ValueType *Typ, int MakeWritable);
struct Value *VariableDefineButIgnoreIdentical(struct ParseState *Parser, char *Ident, struct ValueType *Typ, int IsStatic, int *FirstVisit);
int VariableDefined(Picoc *pc, const char *Ident);
int VariableDefinedAndOutOfScope(Picoc *pc, const char *Ident);
void VariableRealloc(struct ParseState *Parser, struct Value *FromValue, int NewSize);
void VariableGet(Picoc *pc, struct ParseState *Parser, const char *Ident, struct Value **LVal);
void VariableDefinePlatformVar(Picoc *pc, struct ParseState *Parser, char *Ident, struct ValueType *Typ, union AnyValue *FromValue, int IsWritable);
void VariableStackFrameAdd(struct ParseState *Parser, const char *FuncName, int NumParams);
void VariableStackFramePop(struct ParseState *Parser);
struct Value *VariableStringLiteralGet(Picoc *pc, char *Ident);
void VariableStringLiteralDefine(Picoc *pc, char *Ident, struct Value *Val);
void *VariableDereferencePointer(struct ParseState *Parser, struct Value *PointerValue, struct Value **DerefVal, int *DerefOffset, struct ValueType **DerefType, int *DerefIsLValue);
int VariableScopeBegin(struct ParseState * Parser, int* PrevScopeID);
void VariableScopeEnd(struct ParseState * Parser, int ScopeID, int PrevScopeID);


void BasicIOInit(Picoc *pc);
void LibraryInit(Picoc *pc);
void LibraryAdd(Picoc *pc, struct Table *GlobalTable, const char *LibraryName, struct LibraryFunction *FuncList);
void CLibraryInit(Picoc *pc);
void PrintCh(char OutCh, IOFILE *Stream);
void PrintSimpleInt(long Num, IOFILE *Stream);
void PrintInt(long Num, int FieldWidth, int ZeroPad, int LeftJustify, IOFILE *Stream);
void PrintStr(const char *Str, IOFILE *Stream);
//void PrintFP(double Num, IOFILE *Stream);
void PrintType(struct ValueType *Typ, IOFILE *Stream);
void LibPrintf(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs);
//# 589 "c-demos/gitlab.com-zsaleeba-picoc/interpreter.h"
void ProgramFail(struct ParseState *Parser, const char *Message, ...);
void ProgramFailNoParser(Picoc *pc, const char *Message, ...);
void AssignFail(struct ParseState *Parser, const char *Format, struct ValueType *Type1, struct ValueType *Type2, int Num1, int Num2, const char *FuncName, int ParamNo);
void LexFail(Picoc *pc, struct LexState *Lexer, const char *Message, ...);
void PlatformInit(Picoc *pc);
void PlatformCleanup(Picoc *pc);
char *PlatformGetLine(char *Buf, int MaxLen, const char *Prompt);
int PlatformGetCharacter();
void PlatformPutc(unsigned char OutCh, union OutputStreamInfo *);
void PlatformPrintf(IOFILE *Stream, const char *Format, ...);
void PlatformVPrintf(IOFILE *Stream, const char *Format, va_list Args);
void PlatformExit(Picoc *pc, int ExitVal);
char *PlatformMakeTempName(Picoc *pc, char *TempNameBuffer);
void PlatformLibraryInit(Picoc *pc);


void IncludeInit(Picoc *pc);
void IncludeCleanup(Picoc *pc);
void IncludeRegister(Picoc *pc, const char *IncludeName, void (*SetupFunction)(Picoc *pc), struct LibraryFunction *FuncList, const char *SetupCSource);
void IncludeFile(Picoc *pc, char *Filename);




void DebugInit();
void DebugCleanup();
void DebugCheckStatement(struct ParseState *Parser);



extern const char StdioDefs[];
extern struct LibraryFunction StdioFunctions[];
void StdioSetupFunc(Picoc *pc);


extern struct LibraryFunction MathFunctions[];
void MathSetupFunc(Picoc *pc);


extern struct LibraryFunction StringFunctions[];
void StringSetupFunc(Picoc *pc);


extern struct LibraryFunction StdlibFunctions[];
void StdlibSetupFunc(Picoc *pc);


extern const char StdTimeDefs[];
extern struct LibraryFunction StdTimeFunctions[];
void StdTimeSetupFunc(Picoc *pc);


void StdErrnoSetupFunc(Picoc *pc);


extern struct LibraryFunction StdCtypeFunctions[];


extern const char StdboolDefs[];
void StdboolSetupFunc(Picoc *pc);


extern const char UnistdDefs[];
extern struct LibraryFunction UnistdFunctions[];
void UnistdSetupFunc(Picoc *pc);
//# 20 "c-demos/gitlab.com-zsaleeba-picoc/picoc.h" 2
//# 37 "c-demos/gitlab.com-zsaleeba-picoc/picoc.h"
void PicocParse(Picoc *pc, const char *FileName, const char *Source, int SourceLen, int RunIt, int CleanupNow, int CleanupSource, int EnableDebugger);
void PicocParseInteractive(Picoc *pc);


void PicocCallMain(Picoc *pc, int argc, char **argv);
void PicocInitialise(Picoc *pc, int StackSize);
void PicocCleanup(Picoc *pc);
void PicocPlatformScanFile(Picoc *pc, const char *FileName);


void PicocIncludeAllSystemHeaders(Picoc *pc);
//# 5 "c-demos/gitlab.com-zsaleeba-picoc/clibrary.c" 2


////////// nando /////////

char PlatformGetLineBuf[302];
char *PlatformGetLine(char *Buf, int MaxLen, const char *Prompt) {
    for (int i = 0; i <300; i++) {
        int c = getchar();
        if (c < 0) return (char*)0;
        PlatformGetLineBuf[i] = c;
        PlatformGetLineBuf[i+1] = 0;
        if (c == '\n') return PlatformGetLineBuf;
    }
}

void PlatformExit(Picoc *pc, int ExitVal) {
    exit(ExitVal);
}

void PlatformPutc(unsigned char OutCh, union OutputStreamInfo *) {
    putchar(OutCh);
}

////////// nando /////////

// static const int __ENDIAN_CHECK__ = 1;
static int BigEndian = 1;
static int LittleEndian = 0;



void LibraryInit(Picoc *pc)
{


    pc->VersionString = TableStrRegister(pc, "v2.2");
    VariableDefinePlatformVar(pc, 0, "PICOC_VERSION", pc->CharPtrType, (union AnyValue *)&pc->VersionString, 0);


    // BigEndian = ((*(char*)&__ENDIAN_CHECK__) == 0);
    // LittleEndian = ((*(char*)&__ENDIAN_CHECK__) == 1);

    VariableDefinePlatformVar(pc, 0, "BIG_ENDIAN", &pc->IntType, (union AnyValue *)&BigEndian, 0);
    VariableDefinePlatformVar(pc, 0, "LITTLE_ENDIAN", &pc->IntType, (union AnyValue *)&LittleEndian, 0);
}


void LibraryAdd(Picoc *pc, struct Table *GlobalTable, const char *LibraryName, struct LibraryFunction *FuncList)
{
    struct ParseState Parser;
    int Count;
    char *Identifier;
    struct ValueType *ReturnType;
    struct Value *NewValue;
    void *Tokens;
    char *IntrinsicName = TableStrRegister(pc, "c library");


    for (Count = 0; FuncList[Count].Prototype != 0; Count++)
    {
        Tokens = LexAnalyse(pc, IntrinsicName, FuncList[Count].Prototype, strlen((char *)FuncList[Count].Prototype), 0);
        LexInitParser(&Parser, pc, FuncList[Count].Prototype, Tokens, IntrinsicName, 1, 0);
        TypeParse(&Parser, &ReturnType, &Identifier, 0);
        NewValue = ParseFunctionDefinition(&Parser, ReturnType, Identifier);
        NewValue->Val->FuncDef.Intrinsic = FuncList[Count].Func;
        HeapFreeMem(pc, Tokens);
    }
}


void PrintType(struct ValueType *Typ, IOFILE *Stream)
{
    switch (Typ->Base)
    {
        case TypeVoid: PrintStr("void", Stream); break;
        case TypeInt: PrintStr("int", Stream); break;
        case TypeShort: PrintStr("short", Stream); break;
        case TypeChar: PrintStr("char", Stream); break;
        case TypeLong: PrintStr("long", Stream); break;
        case TypeUnsignedInt: PrintStr("unsigned int", Stream); break;
        case TypeUnsignedShort: PrintStr("unsigned short", Stream); break;
        case TypeUnsignedLong: PrintStr("unsigned long", Stream); break;
        case TypeUnsignedChar: PrintStr("unsigned char", Stream); break;

        //case TypeFP: PrintStr("double", Stream); break;

        case TypeFunction: PrintStr("function", Stream); break;
        case TypeMacro: PrintStr("macro", Stream); break;
        case TypePointer: if (Typ->FromType) PrintType(Typ->FromType, Stream); PrintCh('*', Stream); break;
        case TypeArray: PrintType(Typ->FromType, Stream); PrintCh('[', Stream); if (Typ->ArraySize != 0) PrintSimpleInt(Typ->ArraySize, Stream); PrintCh(']', Stream); break;
        case TypeStruct: PrintStr("struct ", Stream); PrintStr( Typ->Identifier, Stream); break;
        case TypeUnion: PrintStr("union ", Stream); PrintStr(Typ->Identifier, Stream); break;
        case TypeEnum: PrintStr("enum ", Stream); PrintStr(Typ->Identifier, Stream); break;
        case TypeGotoLabel: PrintStr("goto label ", Stream); break;
        case Type_Type: PrintStr("type ", Stream); break;
    }
}
//# 92 "c-demos/gitlab.com-zsaleeba-picoc/clibrary.c"
static int TRUEValue = 1;
static int ZeroValue = 0;

void BasicIOInit(Picoc *pc)
{
    pc->CStdOutBase.Putch = &PlatformPutc;
    pc->CStdOut = &pc->CStdOutBase;
}


void CLibraryInit(Picoc *pc)
{

    VariableDefinePlatformVar(pc, 0, "NULL", &pc->IntType, (union AnyValue *)&ZeroValue, 0);
    VariableDefinePlatformVar(pc, 0, "TRUE", &pc->IntType, (union AnyValue *)&TRUEValue, 0);
    VariableDefinePlatformVar(pc, 0, "FALSE", &pc->IntType, (union AnyValue *)&ZeroValue, 0);
}


void SPutc(unsigned char Ch, union OutputStreamInfo *Stream)
{
    struct StringOutputStream *Out = &Stream->Str;
    *Out->WritePos++ = Ch;
}


void PrintCh(char OutCh, struct OutputStream *Stream)
{
    (*Stream->Putch)(OutCh, &Stream->i);
}


void PrintStr(const char *Str, struct OutputStream *Stream)
{
    while (*Str != 0)
        PrintCh(*Str++, Stream);
}


void PrintRepeatedChar(char ShowChar, int Length, struct OutputStream *Stream)
{
    while (Length-- > 0)
        PrintCh(ShowChar, Stream);
}


void PrintUnsigned(unsigned long Num, unsigned int Base, int FieldWidth, int ZeroPad, int LeftJustify, struct OutputStream *Stream)
{
    char Result[33];
    int ResPos = sizeof(Result);

    Result[--ResPos] = '\0';
    if (Num == 0)
        Result[--ResPos] = '0';

    while (Num > 0)
    {
        unsigned long NextNum = Num / Base;
        unsigned long Digit = Num - NextNum * Base;
        if (Digit < 10)
            Result[--ResPos] = '0' + Digit;
        else
            Result[--ResPos] = 'a' + Digit - 10;

        Num = NextNum;
    }

    if (FieldWidth > 0 && !LeftJustify)
        PrintRepeatedChar(ZeroPad ? '0' : ' ', FieldWidth - (sizeof(Result) - 1 - ResPos), Stream);

    PrintStr(&Result[ResPos], Stream);

    if (FieldWidth > 0 && LeftJustify)
        PrintRepeatedChar(' ', FieldWidth - (sizeof(Result) - 1 - ResPos), Stream);
}


void PrintSimpleInt(long Num, struct OutputStream *Stream)
{
    PrintInt(Num, -1, 0, 0, Stream);
}


void PrintInt(long Num, int FieldWidth, int ZeroPad, int LeftJustify, struct OutputStream *Stream)
{
    if (Num < 0)
    {
        PrintCh('-', Stream);
        Num = -Num;
        if (FieldWidth != 0)
            FieldWidth--;
    }

    PrintUnsigned((unsigned long)Num, 10, FieldWidth, ZeroPad, LeftJustify, Stream);
}


#if 0
void PrintFP(double Num, struct OutputStream *Stream)
{
    int Exponent = 0;
    int MaxDecimal;

    if (Num < 0)
    {
        PrintCh('-', Stream);
        Num = -Num;
    }

    if (Num >= 1e7)
        Exponent = log10(Num);
    else if (Num <= 1e-7 && Num != 0.0)
        Exponent = log10(Num) - 0.999999999;

    Num /= pow(10.0, Exponent);
    PrintInt((long)Num, 0, 0, 0, Stream);
    PrintCh('.', Stream);
    Num = (Num - (long)Num) * 10;
    if (abs(Num) >= 1e-7)
    {
        for (MaxDecimal = 6; MaxDecimal > 0 && abs(Num) >= 1e-7; Num = (Num - (long)(Num + 1e-7)) * 10, MaxDecimal--)
            PrintCh('0' + (long)(Num + 1e-7), Stream);
    }
    else
        PrintCh('0', Stream);

    if (Exponent != 0)
    {
        PrintCh('e', Stream);
        PrintInt(Exponent, 0, 0, 0, Stream);
    }
}
#endif


void GenericPrintf(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs, struct OutputStream *Stream)
{
    char *FPos;
    struct Value *NextArg = Param[0];
    struct ValueType *FormatType;
    int ArgCount = 1;
    int LeftJustify = 0;
    int ZeroPad = 0;
    int FieldWidth = 0;
    char *Format = Param[0]->Val->Pointer;

    for (FPos = Format; *FPos != '\0'; FPos++)
    {
        if (*FPos == '%')
        {
            FPos++;
     FieldWidth = 0;
            if (*FPos == '-')
            {

                LeftJustify = 1;
                FPos++;
            }

            if (*FPos == '0')
            {

                ZeroPad = 1;
                FPos++;
            }


            while (isdigit((int)*FPos))
                FieldWidth = FieldWidth * 10 + (*FPos++ - '0');


            switch (*FPos)
            {
                case 's': FormatType = Parser->pc->CharPtrType; break;
                case 'd': case 'u': case 'x': case 'b': case 'c': FormatType = &Parser->pc->IntType; break;

                //case 'f': FormatType = &Parser->pc->FPType; break;

                case '%': PrintCh('%', Stream); FormatType = 0; break;
                case '\0': FPos--; FormatType = 0; break;
                default: PrintCh(*FPos, Stream); FormatType = 0; break;
            }

            if (FormatType != 0)
            {

                if (ArgCount >= NumArgs)
                    PrintStr("XXX", Stream);
                else
                {
                    NextArg = (struct Value *)((char *)NextArg + (((sizeof(struct Value) + TypeStackSizeValue(NextArg)) + sizeof(char) - 1) & ~(sizeof(char)-1)));
                    if (NextArg->Typ != FormatType &&
                            !((FormatType == &Parser->pc->IntType || *FPos == 'f') && ((((NextArg)->Typ)->Base >= TypeInt && ((NextArg)->Typ)->Base <= TypeUnsignedLong) || ((NextArg)->Typ->Base == TypeFP))) &&
                            !(FormatType == Parser->pc->CharPtrType && (NextArg->Typ->Base == TypePointer ||
                                                             (NextArg->Typ->Base == TypeArray && NextArg->Typ->FromType->Base == TypeChar) ) ) )
                        PrintStr("XXX", Stream);
                    else
                    {
                        switch (*FPos)
                        {
                            case 's':
                            {
                                char *Str;

                                if (NextArg->Typ->Base == TypePointer)
                                    Str = NextArg->Val->Pointer;
                                else
                                    Str = &NextArg->Val->ArrayMem[0];

                                if (Str == 0)
                                    PrintStr("NULL", Stream);
                                else
                                    PrintStr(Str, Stream);
                                break;
                            }
                            case 'd': PrintInt(ExpressionCoerceInteger(NextArg), FieldWidth, ZeroPad, LeftJustify, Stream); break;
                            case 'u': PrintUnsigned(ExpressionCoerceUnsignedInteger(NextArg), 10, FieldWidth, ZeroPad, LeftJustify, Stream); break;
                            case 'x': PrintUnsigned(ExpressionCoerceUnsignedInteger(NextArg), 16, FieldWidth, ZeroPad, LeftJustify, Stream); break;
                            case 'b': PrintUnsigned(ExpressionCoerceUnsignedInteger(NextArg), 2, FieldWidth, ZeroPad, LeftJustify, Stream); break;
                            case 'c': PrintCh(ExpressionCoerceUnsignedInteger(NextArg), Stream); break;

                            // case 'f': PrintFP(ExpressionCoerceFP(NextArg), Stream); break;

                        }
                    }
                }

                ArgCount++;
            }
        }
        else
            PrintCh(*FPos, Stream);
    }
}


void LibPrintf(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    struct OutputStream ConsoleStream;

    ConsoleStream.Putch = &PlatformPutc;
    GenericPrintf(Parser, ReturnValue, Param, NumArgs, &ConsoleStream);
}


void LibSPrintf(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    struct OutputStream StrStream;

    StrStream.Putch = &SPutc;
    StrStream.i.Str.Parser = Parser;
    StrStream.i.Str.WritePos = Param[0]->Val->Pointer;

    GenericPrintf(Parser, ReturnValue, Param+1, NumArgs-1, &StrStream);
    PrintCh(0, &StrStream);
    ReturnValue->Val->Pointer = *Param;
}


void LibGets(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    ReturnValue->Val->Pointer = PlatformGetLine(Param[0]->Val->Pointer, 256, 0);
    if (ReturnValue->Val->Pointer != 0)
    {
        char *EOLPos = strchr(Param[0]->Val->Pointer, '\n');
        if (EOLPos != 0)
            *EOLPos = '\0';
    }
}

void LibGetc(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    ReturnValue->Val->Integer = PlatformGetCharacter();
}

void LibExit(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    PlatformExit((Picoc*)0, Param[0]->Val->Integer);
}
//# 465 "c-demos/gitlab.com-zsaleeba-picoc/clibrary.c"
void LibMalloc(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    ReturnValue->Val->Pointer = malloc(Param[0]->Val->Integer);
}


void LibCalloc(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    ReturnValue->Val->Pointer = calloc(Param[0]->Val->Integer, Param[1]->Val->Integer);
}



void LibRealloc(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    ReturnValue->Val->Pointer = realloc(Param[0]->Val->Pointer, Param[1]->Val->Integer);
}


void LibFree(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    free(Param[0]->Val->Pointer);
}

void LibStrcpy(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    char *To = (char *)Param[0]->Val->Pointer;
    char *From = (char *)Param[1]->Val->Pointer;

    while (*From != '\0')
        *To++ = *From++;

    *To = '\0';
}

void LibStrncpy(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    char *To = (char *)Param[0]->Val->Pointer;
    char *From = (char *)Param[1]->Val->Pointer;
    int Len = Param[2]->Val->Integer;

    for (; *From != '\0' && Len > 0; Len--)
        *To++ = *From++;

    if (Len > 0)
        *To = '\0';
}

void LibStrcmp(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    char *Str1 = (char *)Param[0]->Val->Pointer;
    char *Str2 = (char *)Param[1]->Val->Pointer;
    int StrEnded;

    for (StrEnded = 0; !StrEnded; StrEnded = (*Str1 == '\0' || *Str2 == '\0'), Str1++, Str2++)
    {
         if (*Str1 < *Str2) { ReturnValue->Val->Integer = -1; return; }
         else if (*Str1 > *Str2) { ReturnValue->Val->Integer = 1; return; }
    }

    ReturnValue->Val->Integer = 0;
}

void LibStrncmp(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    char *Str1 = (char *)Param[0]->Val->Pointer;
    char *Str2 = (char *)Param[1]->Val->Pointer;
    int Len = Param[2]->Val->Integer;
    int StrEnded;

    for (StrEnded = 0; !StrEnded && Len > 0; StrEnded = (*Str1 == '\0' || *Str2 == '\0'), Str1++, Str2++, Len--)
    {
         if (*Str1 < *Str2) { ReturnValue->Val->Integer = -1; return; }
         else if (*Str1 > *Str2) { ReturnValue->Val->Integer = 1; return; }
    }

    ReturnValue->Val->Integer = 0;
}

void LibStrcat(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    char *To = (char *)Param[0]->Val->Pointer;
    char *From = (char *)Param[1]->Val->Pointer;

    while (*To != '\0')
        To++;

    while (*From != '\0')
        *To++ = *From++;

    *To = '\0';
}

void LibIndex(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    char *Pos = (char *)Param[0]->Val->Pointer;
    int SearchChar = Param[1]->Val->Integer;

    while (*Pos != '\0' && *Pos != SearchChar)
        Pos++;

    if (*Pos != SearchChar)
        ReturnValue->Val->Pointer = 0;
    else
        ReturnValue->Val->Pointer = Pos;
}

void LibRindex(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    char *Pos = (char *)Param[0]->Val->Pointer;
    int SearchChar = Param[1]->Val->Integer;

    ReturnValue->Val->Pointer = 0;
    for (; *Pos != '\0'; Pos++)
    {
        if (*Pos == SearchChar)
            ReturnValue->Val->Pointer = Pos;
    }
}

void LibStrlen(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    char *Pos = (char *)Param[0]->Val->Pointer;
    int Len;

    for (Len = 0; *Pos != '\0'; Pos++)
        Len++;

    ReturnValue->Val->Integer = Len;
}

void LibMemset(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{

    memset(Param[0]->Val->Pointer, Param[1]->Val->Integer, Param[2]->Val->Integer);
}

void LibMemcpy(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{

    memcpy(Param[0]->Val->Pointer, Param[1]->Val->Pointer, Param[2]->Val->Integer);
}

void LibMemcmp(struct ParseState *Parser, struct Value *ReturnValue, struct Value **Param, int NumArgs)
{
    unsigned char *Mem1 = (unsigned char *)Param[0]->Val->Pointer;
    unsigned char *Mem2 = (unsigned char *)Param[1]->Val->Pointer;
    int Len = Param[2]->Val->Integer;

    for (; Len > 0; Mem1++, Mem2++, Len--)
    {
         if (*Mem1 < *Mem2) { ReturnValue->Val->Integer = -1; return; }
         else if (*Mem1 > *Mem2) { ReturnValue->Val->Integer = 1; return; }
    }

    ReturnValue->Val->Integer = 0;
}



struct LibraryFunction CLibrary[] =
{
    { LibPrintf, "void printf(char *, ...);" },
    { LibSPrintf, "char *sprintf(char *, char *, ...);" },
    { LibGets, "char *gets(char *);" },
    { LibGetc, "int getchar();" },
    { LibExit, "void exit(int);" },
//# 652 "c-demos/gitlab.com-zsaleeba-picoc/clibrary.c"
    { LibMalloc, "void *malloc(int);" },

    { LibCalloc, "void *calloc(int,int);" },


    { LibRealloc, "void *realloc(void *,int);" },

    { LibFree, "void free(void *);" },

    { LibStrcpy, "void strcpy(char *,char *);" },
    { LibStrncpy, "void strncpy(char *,char *,int);" },
    { LibStrcmp, "int strcmp(char *,char *);" },
    { LibStrncmp, "int strncmp(char *,char *,int);" },
    { LibStrcat, "void strcat(char *,char *);" },
    { LibIndex, "char *index(char *,int);" },
    { LibRindex, "char *rindex(char *,int);" },
    { LibStrlen, "int strlen(char *);" },
    { LibMemset, "void memset(void *,int,int);" },
    { LibMemcpy, "void memcpy(void *,void *,int);" },
    { LibMemcmp, "int memcmp(void *,void *,int);" },

    { 0, 0 }
};
//# 2 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/debug.c" 1
//# 10 "c-demos/gitlab.com-zsaleeba-picoc/debug.c"
void DebugInit(Picoc *pc)
{
    TableInitTable(&pc->BreakpointTable, &pc->BreakpointHashTable[0], 21, 1);
    pc->BreakpointCount = 0;
}


void DebugCleanup(Picoc *pc)
{
    struct TableEntry *Entry;
    struct TableEntry *NextEntry;
    int Count;

    for (Count = 0; Count < pc->BreakpointTable.Size; Count++)
    {
        for (Entry = pc->BreakpointHashTable[Count]; Entry != 0; Entry = NextEntry)
        {
            NextEntry = Entry->Next;
            HeapFreeMem(pc, Entry);
        }
    }
}


static struct TableEntry *DebugTableSearchBreakpoint(struct ParseState *Parser, int *AddAt)
{
    struct TableEntry *Entry;
    Picoc *pc = Parser->pc;
    int HashValue = ( ((unsigned long)(Parser)->FileName) ^ (((Parser)->Line << 16) | ((Parser)->CharacterPos << 16)) ) % pc->BreakpointTable.Size;

    for (Entry = pc->BreakpointHashTable[HashValue]; Entry != 0; Entry = Entry->Next)
    {
        if (Entry->p.b.FileName == Parser->FileName && Entry->p.b.Line == Parser->Line && Entry->p.b.CharacterPos == Parser->CharacterPos)
            return Entry;
    }

    *AddAt = HashValue;
    return 0;
}


void DebugSetBreakpoint(struct ParseState *Parser)
{
    int AddAt;
    struct TableEntry *FoundEntry = DebugTableSearchBreakpoint(Parser, &AddAt);
    Picoc *pc = Parser->pc;

    if (FoundEntry == 0)
    {

        struct TableEntry *NewEntry = HeapAllocMem(pc, sizeof(struct TableEntry));
        if (NewEntry == 0)
            ProgramFailNoParser(pc, "out of memory");

        NewEntry->p.b.FileName = Parser->FileName;
        NewEntry->p.b.Line = Parser->Line;
        NewEntry->p.b.CharacterPos = Parser->CharacterPos;
        NewEntry->Next = pc->BreakpointHashTable[AddAt];
        pc->BreakpointHashTable[AddAt] = NewEntry;
        pc->BreakpointCount++;
    }
}


int DebugClearBreakpoint(struct ParseState *Parser)
{
    struct TableEntry **EntryPtr;
    Picoc *pc = Parser->pc;
    int HashValue = ( ((unsigned long)(Parser)->FileName) ^ (((Parser)->Line << 16) | ((Parser)->CharacterPos << 16)) ) % pc->BreakpointTable.Size;

    for (EntryPtr = &pc->BreakpointHashTable[HashValue]; *EntryPtr != 0; EntryPtr = &(*EntryPtr)->Next)
    {
        struct TableEntry *DeleteEntry = *EntryPtr;
        if (DeleteEntry->p.b.FileName == Parser->FileName && DeleteEntry->p.b.Line == Parser->Line && DeleteEntry->p.b.CharacterPos == Parser->CharacterPos)
        {
            *EntryPtr = DeleteEntry->Next;
            HeapFreeMem(pc, DeleteEntry);
            pc->BreakpointCount--;

            return 1;
        }
    }

    return 0;
}


void DebugCheckStatement(struct ParseState *Parser)
{
    int DoBreak = 0;
    int AddAt;
    Picoc *pc = Parser->pc;


    if (pc->DebugManualBreak)
    {
        PlatformPrintf(pc->CStdOut, "break\n");
        DoBreak = 1;
        pc->DebugManualBreak = 0;
    }


    if (Parser->pc->BreakpointCount != 0 && DebugTableSearchBreakpoint(Parser, &AddAt) != 0)
        DoBreak = 1;


    if (DoBreak)
    {
        PlatformPrintf(pc->CStdOut, "Handling a break\n");
        PicocParseInteractiveNoStartPrompt(pc, 0);
    }
}

void DebugStep()
{
}
//# 3 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/expression.c" 1
//# 20 "c-demos/gitlab.com-zsaleeba-picoc/expression.c"
void debugf(char *Format, ...)
{
}



enum OperatorOrder
{
    OrderNone,
    OrderPrefix,
    OrderInfix,
    OrderPostfix
};


struct ExpressionStack
{
    struct ExpressionStack *Next;
    struct Value *Val;
    enum LexToken Op;
    int Precedence;
    unsigned char Order;
};


struct OpPrecedence
{
    unsigned int PrefixPrecedence:4;
    unsigned int PostfixPrecedence:4;
    unsigned int InfixPrecedence:4;
    char *Name;
};


static struct OpPrecedence OperatorPrecedence[] =
{
                     { 0, 0, 0, "none" },
                      { 0, 0, 0, "," },
                       { 0, 0, 2, "=" }, { 0, 0, 2, "+=" }, { 0, 0, 2, "-=" },
                               { 0, 0, 2, "*=" }, { 0, 0, 2, "/=" }, { 0, 0, 2, "%=" },
                                { 0, 0, 2, "<<=" }, { 0, 0, 2, ">>=" }, { 0, 0, 2, "&=" },
                                   { 0, 0, 2, "|=" }, { 0, 0, 2, "^=" },
                             { 0, 0, 3, "?" }, { 0, 0, 3, ":" },
                          { 0, 0, 4, "||" },
                           { 0, 0, 5, "&&" },
                             { 0, 0, 6, "|" },
                               { 0, 0, 7, "^" },
                          { 14, 0, 8, "&" },
                       { 0, 0, 9, "==" }, { 0, 0, 9, "!=" },
                         { 0, 0, 10, "<" }, { 0, 0, 10, ">" }, { 0, 0, 10, "<=" }, { 0, 0, 10, ">=" },
                          { 0, 0, 11, "<<" }, { 0, 0, 11, ">>" },
                     { 14, 0, 12, "+" }, { 14, 0, 12, "-" },
                         { 14, 0, 13, "*" }, { 0, 0, 13, "/" }, { 0, 0, 13, "%" },
                          { 14, 15, 0, "++" }, { 14, 15, 0, "--" }, { 14, 0, 0, "!" }, { 14, 0, 0, "~" }, { 14, 0, 0, "sizeof" }, { 14, 0, 0, "cast" },
                                  { 0, 0, 15, "[" }, { 0, 15, 0, "]" }, { 0, 0, 15, "." }, { 0, 0, 15, "->" },
                            { 15, 0, 0, "(" }, { 0, 15, 0, ")" }
};

void ExpressionParseFunctionCall(struct ParseState *Parser, struct ExpressionStack **StackTop, const char *FuncName, int RunIt);
//# 144 "c-demos/gitlab.com-zsaleeba-picoc/expression.c"
int IsTypeToken(struct ParseState * Parser, enum LexToken t, struct Value * LexValue)
{
    if (t >= TokenIntType && t <= TokenUnsignedType)
        return 1;


    if (t == TokenIdentifier)
    {
        struct Value * VarValue;
        if (VariableDefined(Parser->pc, LexValue->Val->Pointer))
        {
            VariableGet(Parser->pc, Parser, LexValue->Val->Pointer, &VarValue);
            if (VarValue->Typ == &Parser->pc->TypeType)
                return 1;
        }
    }

    return 0;
}

long ExpressionCoerceInteger(struct Value *Val)
{
    switch (Val->Typ->Base)
    {
        case TypeInt: return (long)Val->Val->Integer;
        case TypeChar: return (long)Val->Val->Character;
        case TypeShort: return (long)Val->Val->ShortInteger;
        case TypeLong: return (long)Val->Val->LongInteger;
        case TypeUnsignedInt: return (long)Val->Val->UnsignedInteger;
        case TypeUnsignedShort: return (long)Val->Val->UnsignedShortInteger;
        case TypeUnsignedLong: return (long)Val->Val->UnsignedLongInteger;
        case TypeUnsignedChar: return (long)Val->Val->UnsignedCharacter;
        case TypePointer: return (long)Val->Val->Pointer;

        // case TypeFP: return (long)Val->Val->FP;

        default: return 0;
    }
}

unsigned long ExpressionCoerceUnsignedInteger(struct Value *Val)
{
    switch (Val->Typ->Base)
    {
        case TypeInt: return (unsigned long)Val->Val->Integer;
        case TypeChar: return (unsigned long)Val->Val->Character;
        case TypeShort: return (unsigned long)Val->Val->ShortInteger;
        case TypeLong: return (unsigned long)Val->Val->LongInteger;
        case TypeUnsignedInt: return (unsigned long)Val->Val->UnsignedInteger;
        case TypeUnsignedShort: return (unsigned long)Val->Val->UnsignedShortInteger;
        case TypeUnsignedLong: return (unsigned long)Val->Val->UnsignedLongInteger;
        case TypeUnsignedChar: return (unsigned long)Val->Val->UnsignedCharacter;
        case TypePointer: return (unsigned long)Val->Val->Pointer;

        // case TypeFP: return (unsigned long)Val->Val->FP;

        default: return 0;
    }
}


#if 0
double ExpressionCoerceFP(struct Value *Val)
{

    int IntVal;
    unsigned UnsignedVal;

    switch (Val->Typ->Base)
    {
        case TypeInt: IntVal = Val->Val->Integer; return (double)IntVal;
        case TypeChar: IntVal = Val->Val->Character; return (double)IntVal;
        case TypeShort: IntVal = Val->Val->ShortInteger; return (double)IntVal;
        case TypeLong: IntVal = Val->Val->LongInteger; return (double)IntVal;
        case TypeUnsignedInt: UnsignedVal = Val->Val->UnsignedInteger; return (double)UnsignedVal;
        case TypeUnsignedShort: UnsignedVal = Val->Val->UnsignedShortInteger; return (double)UnsignedVal;
        case TypeUnsignedLong: UnsignedVal = Val->Val->UnsignedLongInteger; return (double)UnsignedVal;
        case TypeUnsignedChar: UnsignedVal = Val->Val->UnsignedCharacter; return (double)UnsignedVal;
        // case TypeFP: return Val->Val->FP;
        default: return 0.0;
    }
//# 239 "c-demos/gitlab.com-zsaleeba-picoc/expression.c"
}
#endif



long ExpressionAssignInt(struct ParseState *Parser, struct Value *DestValue, long FromInt, int After)
{
    long Result;

    if (!DestValue->IsLValue)
        ProgramFail(Parser, "can't assign to this");

    if (After)
        Result = ExpressionCoerceInteger(DestValue);
    else
        Result = FromInt;

    switch (DestValue->Typ->Base)
    {
        case TypeInt: DestValue->Val->Integer = FromInt; break;
        case TypeShort: DestValue->Val->ShortInteger = (short)FromInt; break;
        case TypeChar: DestValue->Val->Character = (char)FromInt; break;
        case TypeLong: DestValue->Val->LongInteger = (long)FromInt; break;
        case TypeUnsignedInt: DestValue->Val->UnsignedInteger = (unsigned int)FromInt; break;
        case TypeUnsignedShort: DestValue->Val->UnsignedShortInteger = (unsigned short)FromInt; break;
        case TypeUnsignedLong: DestValue->Val->UnsignedLongInteger = (unsigned long)FromInt; break;
        case TypeUnsignedChar: DestValue->Val->UnsignedCharacter = (unsigned char)FromInt; break;
        default: break;
    }
    return Result;
}


#if 0
double ExpressionAssignFP(struct ParseState *Parser, struct Value *DestValue, double FromFP)
{
    if (!DestValue->IsLValue)
        ProgramFail(Parser, "can't assign to this");

    DestValue->Val->FP = FromFP;
    return FromFP;
}
#endif



void ExpressionStackPushValueNode(struct ParseState *Parser, struct ExpressionStack **StackTop, struct Value *ValueLoc)
{
    struct ExpressionStack *StackNode = VariableAlloc(Parser->pc, Parser, sizeof(struct ExpressionStack), 0);
    StackNode->Next = *StackTop;
    StackNode->Val = ValueLoc;
    *StackTop = StackNode;







}


struct Value *ExpressionStackPushValueByType(struct ParseState *Parser, struct ExpressionStack **StackTop, struct ValueType *PushType)
{
    struct Value *ValueLoc = VariableAllocValueFromType(Parser->pc, Parser, PushType, 0, 0, 0);
    ExpressionStackPushValueNode(Parser, StackTop, ValueLoc);

    return ValueLoc;
}


void ExpressionStackPushValue(struct ParseState *Parser, struct ExpressionStack **StackTop, struct Value *PushValue)
{
    struct Value *ValueLoc = VariableAllocValueAndCopy(Parser->pc, Parser, PushValue, 0);
    ExpressionStackPushValueNode(Parser, StackTop, ValueLoc);
}

void ExpressionStackPushLValue(struct ParseState *Parser, struct ExpressionStack **StackTop, struct Value *PushValue, int Offset)
{
    struct Value *ValueLoc = VariableAllocValueShared(Parser, PushValue);
    ValueLoc->Val = (void *)((char *)ValueLoc->Val + Offset);
    ExpressionStackPushValueNode(Parser, StackTop, ValueLoc);
}

void ExpressionStackPushDereference(struct ParseState *Parser, struct ExpressionStack **StackTop, struct Value *DereferenceValue)
{
    struct Value *DerefVal;
    struct Value *ValueLoc;
    int Offset;
    struct ValueType *DerefType;
    int DerefIsLValue;
    void *DerefDataLoc = VariableDereferencePointer(Parser, DereferenceValue, &DerefVal, &Offset, &DerefType, &DerefIsLValue);
    if (DerefDataLoc == 0)
        ProgramFail(Parser, "NULL pointer dereference");

    ValueLoc = VariableAllocValueFromExistingData(Parser, DerefType, (union AnyValue *)DerefDataLoc, DerefIsLValue, DerefVal);
    ExpressionStackPushValueNode(Parser, StackTop, ValueLoc);
}

void ExpressionPushInt(struct ParseState *Parser, struct ExpressionStack **StackTop, long IntValue)
{
    struct Value *ValueLoc = VariableAllocValueFromType(Parser->pc, Parser, &Parser->pc->IntType, 0, 0, 0);
    ValueLoc->Val->Integer = IntValue;
    ExpressionStackPushValueNode(Parser, StackTop, ValueLoc);
}

#if 0
void ExpressionPushFP(struct ParseState *Parser, struct ExpressionStack **StackTop, double FPValue)
{
    struct Value *ValueLoc = VariableAllocValueFromType(Parser->pc, Parser, &Parser->pc->FPType, 0, 0, 0);
    ValueLoc->Val->FP = FPValue;
    ExpressionStackPushValueNode(Parser, StackTop, ValueLoc);
}
#endif


void ExpressionAssignToPointer(struct ParseState *Parser, struct Value *ToValue, struct Value *FromValue, const char *FuncName, int ParamNo, int AllowPointerCoercion)
{
    struct ValueType *PointedToType = ToValue->Typ->FromType;

    if (FromValue->Typ == ToValue->Typ || FromValue->Typ == Parser->pc->VoidPtrType || (ToValue->Typ == Parser->pc->VoidPtrType && FromValue->Typ->Base == TypePointer))
        ToValue->Val->Pointer = FromValue->Val->Pointer;

    else if (FromValue->Typ->Base == TypeArray && (PointedToType == FromValue->Typ->FromType || ToValue->Typ == Parser->pc->VoidPtrType))
    {

        ToValue->Val->Pointer = (void *)&FromValue->Val->ArrayMem[0];
    }
    else if (FromValue->Typ->Base == TypePointer && FromValue->Typ->FromType->Base == TypeArray &&
               (PointedToType == FromValue->Typ->FromType->FromType || ToValue->Typ == Parser->pc->VoidPtrType) )
    {

        ToValue->Val->Pointer = VariableDereferencePointer(Parser, FromValue, 0, 0, 0, 0);
    }
    else if (((((FromValue)->Typ)->Base >= TypeInt && ((FromValue)->Typ)->Base <= TypeUnsignedLong) /*|| ((FromValue)->Typ->Base == TypeFP)) && ExpressionCoerceInteger(FromValue) == 0*/))
    {

        ToValue->Val->Pointer = 0;
    }
    else if (AllowPointerCoercion && ((((FromValue)->Typ)->Base >= TypeInt && ((FromValue)->Typ)->Base <= TypeUnsignedLong) /*|| ((FromValue)->Typ->Base == TypeFP)*/))
    {

        ToValue->Val->Pointer = (void *)(unsigned long)ExpressionCoerceUnsignedInteger(FromValue);
    }
    else if (AllowPointerCoercion && FromValue->Typ->Base == TypePointer)
    {

        ToValue->Val->Pointer = FromValue->Val->Pointer;
    }
    else
        AssignFail(Parser, "%t from %t", ToValue->Typ, FromValue->Typ, 0, 0, FuncName, ParamNo);
}


void ExpressionAssign(struct ParseState *Parser, struct Value *DestValue, struct Value *SourceValue, int Force, const char *FuncName, int ParamNo, int AllowPointerCoercion)
{
    if (!DestValue->IsLValue && !Force)
        AssignFail(Parser, "not an lvalue", 0, 0, 0, 0, FuncName, ParamNo);

    if (((((DestValue)->Typ)->Base >= TypeInt && ((DestValue)->Typ)->Base <= TypeUnsignedLong) || ((DestValue)->Typ->Base == TypeFP)) && !(((((SourceValue)->Typ)->Base >= TypeInt && ((SourceValue)->Typ)->Base <= TypeUnsignedLong) || ((SourceValue)->Typ->Base == TypeFP)) || ((AllowPointerCoercion) ? ((SourceValue)->Typ->Base == TypePointer) : 0)))
        AssignFail(Parser, "%t from %t", DestValue->Typ, SourceValue->Typ, 0, 0, FuncName, ParamNo);

    switch (DestValue->Typ->Base)
    {
        case TypeInt: DestValue->Val->Integer = ExpressionCoerceInteger(SourceValue); break;
        case TypeShort: DestValue->Val->ShortInteger = (short)ExpressionCoerceInteger(SourceValue); break;
        case TypeChar: DestValue->Val->Character = (char)ExpressionCoerceInteger(SourceValue); break;
        case TypeLong: DestValue->Val->LongInteger = ExpressionCoerceInteger(SourceValue); break;
        case TypeUnsignedInt: DestValue->Val->UnsignedInteger = ExpressionCoerceUnsignedInteger(SourceValue); break;
        case TypeUnsignedShort: DestValue->Val->UnsignedShortInteger = (unsigned short)ExpressionCoerceUnsignedInteger(SourceValue); break;
        case TypeUnsignedLong: DestValue->Val->UnsignedLongInteger = ExpressionCoerceUnsignedInteger(SourceValue); break;
        case TypeUnsignedChar: DestValue->Val->UnsignedCharacter = (unsigned char)ExpressionCoerceUnsignedInteger(SourceValue); break;

#if 0
        case TypeFP:
            if (!(((((SourceValue)->Typ)->Base >= TypeInt && ((SourceValue)->Typ)->Base <= TypeUnsignedLong) || ((SourceValue)->Typ->Base == TypeFP)) || ((AllowPointerCoercion) ? ((SourceValue)->Typ->Base == TypePointer) : 0)))
                AssignFail(Parser, "%t from %t", DestValue->Typ, SourceValue->Typ, 0, 0, FuncName, ParamNo);

            DestValue->Val->FP = ExpressionCoerceFP(SourceValue);
            break;
#endif
        case TypePointer:
            ExpressionAssignToPointer(Parser, DestValue, SourceValue, FuncName, ParamNo, AllowPointerCoercion);
            break;

        case TypeArray:
            if (SourceValue->Typ->Base == TypeArray && DestValue->Typ->FromType == DestValue->Typ->FromType && DestValue->Typ->ArraySize == 0)
            {

                DestValue->Typ = SourceValue->Typ;
                VariableRealloc(Parser, DestValue, TypeSizeValue(DestValue, 0));

                if (DestValue->LValueFrom != 0)
                {

                    DestValue->LValueFrom->Val = DestValue->Val;
                    DestValue->LValueFrom->AnyValOnHeap = DestValue->AnyValOnHeap;
                }
            }


            if (DestValue->Typ->FromType->Base == TypeChar && SourceValue->Typ->Base == TypePointer && SourceValue->Typ->FromType->Base == TypeChar)
            {
                if (DestValue->Typ->ArraySize == 0)
                {
                    int Size = strlen(SourceValue->Val->Pointer) + 1;




                    DestValue->Typ = TypeGetMatching(Parser->pc, Parser, DestValue->Typ->FromType, DestValue->Typ->Base, Size, DestValue->Typ->Identifier, 1);
                    VariableRealloc(Parser, DestValue, TypeSizeValue(DestValue, 0));
                }






                memcpy((void *)DestValue->Val, SourceValue->Val->Pointer, TypeSizeValue(DestValue, 0));
                break;
            }

            if (DestValue->Typ != SourceValue->Typ)
                AssignFail(Parser, "%t from %t", DestValue->Typ, SourceValue->Typ, 0, 0, FuncName, ParamNo);

            if (DestValue->Typ->ArraySize != SourceValue->Typ->ArraySize)
                AssignFail(Parser, "from an array of size %d to one of size %d", 0, 0, DestValue->Typ->ArraySize, SourceValue->Typ->ArraySize, FuncName, ParamNo);

            memcpy((void *)DestValue->Val, (void *)SourceValue->Val, TypeSizeValue(DestValue, 0));
            break;

        case TypeStruct:
        case TypeUnion:
            if (DestValue->Typ != SourceValue->Typ)
                AssignFail(Parser, "%t from %t", DestValue->Typ, SourceValue->Typ, 0, 0, FuncName, ParamNo);

            memcpy((void *)DestValue->Val, (void *)SourceValue->Val, TypeSizeValue(SourceValue, 0));
            break;

        default:
            AssignFail(Parser, "%t", DestValue->Typ, 0, 0, 0, FuncName, ParamNo);
            break;
    }
}


void ExpressionQuestionMarkOperator(struct ParseState *Parser, struct ExpressionStack **StackTop, struct Value *BottomValue, struct Value *TopValue)
{
    if (!((((TopValue)->Typ)->Base >= TypeInt && ((TopValue)->Typ)->Base <= TypeUnsignedLong) || ((TopValue)->Typ->Base == TypeFP)))
        ProgramFail(Parser, "first argument to '?' should be a number");

    if (ExpressionCoerceInteger(TopValue))
    {

        ExpressionStackPushValue(Parser, StackTop, BottomValue);
    }
    else
    {

        ExpressionStackPushValueByType(Parser, StackTop, &Parser->pc->VoidType);
    }
}


void ExpressionColonOperator(struct ParseState *Parser, struct ExpressionStack **StackTop, struct Value *BottomValue, struct Value *TopValue)
{
    if (TopValue->Typ->Base == TypeVoid)
    {

        ExpressionStackPushValue(Parser, StackTop, BottomValue);
    }
    else
    {

        ExpressionStackPushValue(Parser, StackTop, TopValue);
    }
}


void ExpressionPrefixOperator(struct ParseState *Parser, struct ExpressionStack **StackTop, enum LexToken Op, struct Value *TopValue)
{
    struct Value *Result;
    union AnyValue *ValPtr;

    debugf("ExpressionPrefixOperator()\n");
    switch (Op)
    {
        case TokenAmpersand:
            if (!TopValue->IsLValue)
                ProgramFail(Parser, "can't get the address of this");

     ValPtr = TopValue->Val;
            Result = VariableAllocValueFromType(Parser->pc, Parser, TypeGetMatching(Parser->pc, Parser, TopValue->Typ, TypePointer, 0, Parser->pc->StrEmpty, 1), 0, 0, 0);
            Result->Val->Pointer = (void *)ValPtr;
            ExpressionStackPushValueNode(Parser, StackTop, Result);
            break;

        case TokenAsterisk:
            ExpressionStackPushDereference(Parser, StackTop, TopValue);
            break;

        case TokenSizeof:

            if (TopValue->Typ == &Parser->pc->TypeType)
                ExpressionPushInt(Parser, StackTop, TypeSize(TopValue->Val->Typ, TopValue->Val->Typ->ArraySize, 1));
            else
                ExpressionPushInt(Parser, StackTop, TypeSize(TopValue->Typ, TopValue->Typ->ArraySize, 1));
            break;

        default:

#if 0
            if (TopValue->Typ == &Parser->pc->FPType)
            {

                double ResultFP = 0.0;

                switch (Op)
                {
                    case TokenPlus: ResultFP = TopValue->Val->FP; break;
                    case TokenMinus: ResultFP = -TopValue->Val->FP; break;
                    case TokenIncrement: ResultFP = ExpressionAssignFP(Parser, TopValue, TopValue->Val->FP+1); break;
                    case TokenDecrement: ResultFP = ExpressionAssignFP(Parser, TopValue, TopValue->Val->FP-1); break;
                    case TokenUnaryNot: ResultFP = !TopValue->Val->FP; break;
                    default: ProgramFail(Parser, "invalid operation"); break;
                }

                ExpressionPushFP(Parser, StackTop, ResultFP);
            }
            else
#endif
            if (((((TopValue)->Typ)->Base >= TypeInt && ((TopValue)->Typ)->Base <= TypeUnsignedLong) || ((TopValue)->Typ->Base == TypeFP)))
            {

                long ResultInt = 0;
                long TopInt = ExpressionCoerceInteger(TopValue);
                switch (Op)
                {
                    case TokenPlus: ResultInt = TopInt; break;
                    case TokenMinus: ResultInt = -TopInt; break;
                    case TokenIncrement: ResultInt = ExpressionAssignInt(Parser, TopValue, TopInt+1, 0); break;
                    case TokenDecrement: ResultInt = ExpressionAssignInt(Parser, TopValue, TopInt-1, 0); break;
                    case TokenUnaryNot: ResultInt = !TopInt; break;
                    case TokenUnaryExor: ResultInt = ~TopInt; break;
                    default: ProgramFail(Parser, "invalid operation"); break;
                }

                ExpressionPushInt(Parser, StackTop, ResultInt);
            }
            else if (TopValue->Typ->Base == TypePointer)
            {

                int Size = TypeSize(TopValue->Typ->FromType, 0, 1);
                struct Value *StackValue;
                void *ResultPtr;

                if (TopValue->Val->Pointer == 0)
                    ProgramFail(Parser, "invalid use of a NULL pointer");

                if (!TopValue->IsLValue)
                    ProgramFail(Parser, "can't assign to this");

                switch (Op)
                {
                    case TokenIncrement: TopValue->Val->Pointer = (void *)((char *)TopValue->Val->Pointer + Size); break;
                    case TokenDecrement: TopValue->Val->Pointer = (void *)((char *)TopValue->Val->Pointer - Size); break;
                    default: ProgramFail(Parser, "invalid operation"); break;
                }

                ResultPtr = TopValue->Val->Pointer;
                StackValue = ExpressionStackPushValueByType(Parser, StackTop, TopValue->Typ);
                StackValue->Val->Pointer = ResultPtr;
            }
            else
                ProgramFail(Parser, "invalid operation");
            break;
    }
}


void ExpressionPostfixOperator(struct ParseState *Parser, struct ExpressionStack **StackTop, enum LexToken Op, struct Value *TopValue)
{
    debugf("ExpressionPostfixOperator()\n");
#if 0
    if (TopValue->Typ == &Parser->pc->FPType)
    {

        double ResultFP = 0.0;

        switch (Op)
        {
            case TokenIncrement: ResultFP = ExpressionAssignFP(Parser, TopValue, TopValue->Val->FP+1); break;
            case TokenDecrement: ResultFP = ExpressionAssignFP(Parser, TopValue, TopValue->Val->FP-1); break;
            default: ProgramFail(Parser, "invalid operation"); break;
        }

        ExpressionPushFP(Parser, StackTop, ResultFP);
    }
    else
#endif
    if (((((TopValue)->Typ)->Base >= TypeInt && ((TopValue)->Typ)->Base <= TypeUnsignedLong) || ((TopValue)->Typ->Base == TypeFP)))
    {
        long ResultInt = 0;
        long TopInt = ExpressionCoerceInteger(TopValue);
        switch (Op)
        {
            case TokenIncrement: ResultInt = ExpressionAssignInt(Parser, TopValue, TopInt+1, 1); break;
            case TokenDecrement: ResultInt = ExpressionAssignInt(Parser, TopValue, TopInt-1, 1); break;
            case TokenRightSquareBracket: ProgramFail(Parser, "not supported"); break;
            case TokenCloseBracket: ProgramFail(Parser, "not supported"); break;
            default: ProgramFail(Parser, "invalid operation"); break;
        }

        ExpressionPushInt(Parser, StackTop, ResultInt);
    }
    else if (TopValue->Typ->Base == TypePointer)
    {

        int Size = TypeSize(TopValue->Typ->FromType, 0, 1);
        struct Value *StackValue;
        void *OrigPointer = TopValue->Val->Pointer;

        if (TopValue->Val->Pointer == 0)
            ProgramFail(Parser, "invalid use of a NULL pointer");

        if (!TopValue->IsLValue)
            ProgramFail(Parser, "can't assign to this");

        switch (Op)
        {
            case TokenIncrement: TopValue->Val->Pointer = (void *)((char *)TopValue->Val->Pointer + Size); break;
            case TokenDecrement: TopValue->Val->Pointer = (void *)((char *)TopValue->Val->Pointer - Size); break;
            default: ProgramFail(Parser, "invalid operation"); break;
        }

        StackValue = ExpressionStackPushValueByType(Parser, StackTop, TopValue->Typ);
        StackValue->Val->Pointer = OrigPointer;
    }
    else
        ProgramFail(Parser, "invalid operation");
}


void ExpressionInfixOperator(struct ParseState *Parser, struct ExpressionStack **StackTop, enum LexToken Op, struct Value *BottomValue, struct Value *TopValue)
{
    long ResultInt = 0;
    struct Value *StackValue;
    void *Pointer;

    debugf("ExpressionInfixOperator()\n");
    if (BottomValue == 0 || TopValue == 0)
        ProgramFail(Parser, "invalid expression");

    if (Op == TokenLeftSquareBracket)
    {

        int ArrayIndex;
        struct Value *Result = 0;

        if (!((((TopValue)->Typ)->Base >= TypeInt && ((TopValue)->Typ)->Base <= TypeUnsignedLong) || ((TopValue)->Typ->Base == TypeFP)))
            ProgramFail(Parser, "array index must be an integer");

        ArrayIndex = ExpressionCoerceInteger(TopValue);


        switch (BottomValue->Typ->Base)
        {
            case TypeArray: Result = VariableAllocValueFromExistingData(Parser, BottomValue->Typ->FromType, (union AnyValue *)(&BottomValue->Val->ArrayMem[0] + TypeSize(BottomValue->Typ, ArrayIndex, 1)), BottomValue->IsLValue, BottomValue->LValueFrom); break;
            case TypePointer: Result = VariableAllocValueFromExistingData(Parser, BottomValue->Typ->FromType, (union AnyValue *)((char *)BottomValue->Val->Pointer + TypeSize(BottomValue->Typ->FromType, 0, 1) * ArrayIndex), BottomValue->IsLValue, BottomValue->LValueFrom); break;
            default: ProgramFail(Parser, "this %t is not an array", BottomValue->Typ);
        }

        ExpressionStackPushValueNode(Parser, StackTop, Result);
    }
    else if (Op == TokenQuestionMark)
        ExpressionQuestionMarkOperator(Parser, StackTop, TopValue, BottomValue);

    else if (Op == TokenColon)
        ExpressionColonOperator(Parser, StackTop, TopValue, BottomValue);

#if 0
    else if ( (TopValue->Typ == &Parser->pc->FPType && BottomValue->Typ == &Parser->pc->FPType) ||
              (TopValue->Typ == &Parser->pc->FPType && ((((BottomValue)->Typ)->Base >= TypeInt && ((BottomValue)->Typ)->Base <= TypeUnsignedLong) || ((BottomValue)->Typ->Base == TypeFP))) ||
              (((((TopValue)->Typ)->Base >= TypeInt && ((TopValue)->Typ)->Base <= TypeUnsignedLong) || ((TopValue)->Typ->Base == TypeFP)) && BottomValue->Typ == &Parser->pc->FPType) )
    {

        int ResultIsInt = 0;
        double ResultFP = 0.0;
        double TopFP = (TopValue->Typ == &Parser->pc->FPType) ? TopValue->Val->FP : (double)ExpressionCoerceInteger(TopValue);
        double BottomFP = (BottomValue->Typ == &Parser->pc->FPType) ? BottomValue->Val->FP : (double)ExpressionCoerceInteger(BottomValue);

        switch (Op)
        {
            case TokenAssign: if (((BottomValue)->Typ->Base == TypeFP)) { ResultFP = ExpressionAssignFP(Parser, BottomValue, TopFP); } else { ResultInt = ExpressionAssignInt(Parser, BottomValue, (long)(TopFP), 0); ResultIsInt = 1; }; break;
            case TokenAddAssign: if (((BottomValue)->Typ->Base == TypeFP)) { ResultFP = ExpressionAssignFP(Parser, BottomValue, BottomFP + TopFP); } else { ResultInt = ExpressionAssignInt(Parser, BottomValue, (long)(BottomFP + TopFP), 0); ResultIsInt = 1; }; break;
            case TokenSubtractAssign: if (((BottomValue)->Typ->Base == TypeFP)) { ResultFP = ExpressionAssignFP(Parser, BottomValue, BottomFP - TopFP); } else { ResultInt = ExpressionAssignInt(Parser, BottomValue, (long)(BottomFP - TopFP), 0); ResultIsInt = 1; }; break;
            case TokenMultiplyAssign: if (((BottomValue)->Typ->Base == TypeFP)) { ResultFP = ExpressionAssignFP(Parser, BottomValue, BottomFP * TopFP); } else { ResultInt = ExpressionAssignInt(Parser, BottomValue, (long)(BottomFP * TopFP), 0); ResultIsInt = 1; }; break;
            case TokenDivideAssign: if (((BottomValue)->Typ->Base == TypeFP)) { ResultFP = ExpressionAssignFP(Parser, BottomValue, BottomFP / TopFP); } else { ResultInt = ExpressionAssignInt(Parser, BottomValue, (long)(BottomFP / TopFP), 0); ResultIsInt = 1; }; break;
            case TokenEqual: ResultInt = BottomFP == TopFP; ResultIsInt = 1; break;
            case TokenNotEqual: ResultInt = BottomFP != TopFP; ResultIsInt = 1; break;
            case TokenLessThan: ResultInt = BottomFP < TopFP; ResultIsInt = 1; break;
            case TokenGreaterThan: ResultInt = BottomFP > TopFP; ResultIsInt = 1; break;
            case TokenLessEqual: ResultInt = BottomFP <= TopFP; ResultIsInt = 1; break;
            case TokenGreaterEqual: ResultInt = BottomFP >= TopFP; ResultIsInt = 1; break;
            case TokenPlus: ResultFP = BottomFP + TopFP; break;
            case TokenMinus: ResultFP = BottomFP - TopFP; break;
            case TokenAsterisk: ResultFP = BottomFP * TopFP; break;
            case TokenSlash: ResultFP = BottomFP / TopFP; break;
            default: ProgramFail(Parser, "invalid operation"); break;
        }

        if (ResultIsInt)
            ExpressionPushInt(Parser, StackTop, ResultInt);
        else
            ExpressionPushFP(Parser, StackTop, ResultFP);
    }
#endif
    else if (((((TopValue)->Typ)->Base >= TypeInt && ((TopValue)->Typ)->Base <= TypeUnsignedLong) || ((TopValue)->Typ->Base == TypeFP)) && ((((BottomValue)->Typ)->Base >= TypeInt && ((BottomValue)->Typ)->Base <= TypeUnsignedLong) || ((BottomValue)->Typ->Base == TypeFP)))
    {

        long TopInt = ExpressionCoerceInteger(TopValue);
        long BottomInt = ExpressionCoerceInteger(BottomValue);
        switch (Op)
        {
            case TokenAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, TopInt, 0); break;
            case TokenAddAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt + TopInt, 0); break;
            case TokenSubtractAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt - TopInt, 0); break;
            case TokenMultiplyAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt * TopInt, 0); break;
            case TokenDivideAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt / TopInt, 0); break;

            case TokenModulusAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt % TopInt, 0); break;

            case TokenShiftLeftAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt << TopInt, 0); break;
            case TokenShiftRightAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt >> TopInt, 0); break;
            case TokenArithmeticAndAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt & TopInt, 0); break;
            case TokenArithmeticOrAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt | TopInt, 0); break;
            case TokenArithmeticExorAssign: ResultInt = ExpressionAssignInt(Parser, BottomValue, BottomInt ^ TopInt, 0); break;
            case TokenLogicalOr: ResultInt = BottomInt || TopInt; break;
            case TokenLogicalAnd: ResultInt = BottomInt && TopInt; break;
            case TokenArithmeticOr: ResultInt = BottomInt | TopInt; break;
            case TokenArithmeticExor: ResultInt = BottomInt ^ TopInt; break;
            case TokenAmpersand: ResultInt = BottomInt & TopInt; break;
            case TokenEqual: ResultInt = BottomInt == TopInt; break;
            case TokenNotEqual: ResultInt = BottomInt != TopInt; break;
            case TokenLessThan: ResultInt = BottomInt < TopInt; break;
            case TokenGreaterThan: ResultInt = BottomInt > TopInt; break;
            case TokenLessEqual: ResultInt = BottomInt <= TopInt; break;
            case TokenGreaterEqual: ResultInt = BottomInt >= TopInt; break;
            case TokenShiftLeft: ResultInt = BottomInt << TopInt; break;
            case TokenShiftRight: ResultInt = BottomInt >> TopInt; break;
            case TokenPlus: ResultInt = BottomInt + TopInt; break;
            case TokenMinus: ResultInt = BottomInt - TopInt; break;
            case TokenAsterisk: ResultInt = BottomInt * TopInt; break;
            case TokenSlash: ResultInt = BottomInt / TopInt; break;

            case TokenModulus: ResultInt = BottomInt % TopInt; break;

            default: ProgramFail(Parser, "invalid operation"); break;
        }

        ExpressionPushInt(Parser, StackTop, ResultInt);
    }
    else if (BottomValue->Typ->Base == TypePointer && ((((TopValue)->Typ)->Base >= TypeInt && ((TopValue)->Typ)->Base <= TypeUnsignedLong) || ((TopValue)->Typ->Base == TypeFP)))
    {

        long TopInt = ExpressionCoerceInteger(TopValue);

        if (Op == TokenEqual || Op == TokenNotEqual)
        {

            if (TopInt != 0)
                ProgramFail(Parser, "invalid operation");

            if (Op == TokenEqual)
                ExpressionPushInt(Parser, StackTop, BottomValue->Val->Pointer == 0);
            else
                ExpressionPushInt(Parser, StackTop, BottomValue->Val->Pointer != 0);
        }
        else if (Op == TokenPlus || Op == TokenMinus)
        {

            int Size = TypeSize(BottomValue->Typ->FromType, 0, 1);

            Pointer = BottomValue->Val->Pointer;
            if (Pointer == 0)
                ProgramFail(Parser, "invalid use of a NULL pointer");

            if (Op == TokenPlus)
                Pointer = (void *)((char *)Pointer + TopInt * Size);
            else
                Pointer = (void *)((char *)Pointer - TopInt * Size);

            StackValue = ExpressionStackPushValueByType(Parser, StackTop, BottomValue->Typ);
            StackValue->Val->Pointer = Pointer;
        }
        else if (Op == TokenAssign && TopInt == 0)
        {

            HeapUnpopStack(Parser->pc, sizeof(struct Value));
            ExpressionAssign(Parser, BottomValue, TopValue, 0, 0, 0, 0);
            ExpressionStackPushValueNode(Parser, StackTop, BottomValue);
        }
        else if (Op == TokenAddAssign || Op == TokenSubtractAssign)
        {

            int Size = TypeSize(BottomValue->Typ->FromType, 0, 1);

            Pointer = BottomValue->Val->Pointer;
            if (Pointer == 0)
                ProgramFail(Parser, "invalid use of a NULL pointer");

            if (Op == TokenAddAssign)
                Pointer = (void *)((char *)Pointer + TopInt * Size);
            else
                Pointer = (void *)((char *)Pointer - TopInt * Size);

            HeapUnpopStack(Parser->pc, sizeof(struct Value));
            BottomValue->Val->Pointer = Pointer;
            ExpressionStackPushValueNode(Parser, StackTop, BottomValue);
        }
        else
            ProgramFail(Parser, "invalid operation");
    }
    else if (BottomValue->Typ->Base == TypePointer && TopValue->Typ->Base == TypePointer && Op != TokenAssign)
    {

        char *TopLoc = (char *)TopValue->Val->Pointer;
        char *BottomLoc = (char *)BottomValue->Val->Pointer;

        switch (Op)
        {
            case TokenEqual: ExpressionPushInt(Parser, StackTop, BottomLoc == TopLoc); break;
            case TokenNotEqual: ExpressionPushInt(Parser, StackTop, BottomLoc != TopLoc); break;
            case TokenMinus: ExpressionPushInt(Parser, StackTop, BottomLoc - TopLoc); break;
            default: ProgramFail(Parser, "invalid operation"); break;
        }
    }
    else if (Op == TokenAssign)
    {

        HeapUnpopStack(Parser->pc, sizeof(struct Value));
        ExpressionAssign(Parser, BottomValue, TopValue, 0, 0, 0, 0);
        ExpressionStackPushValueNode(Parser, StackTop, BottomValue);
    }
    else if (Op == TokenCast)
    {

        struct Value *ValueLoc = ExpressionStackPushValueByType(Parser, StackTop, BottomValue->Val->Typ);
        ExpressionAssign(Parser, ValueLoc, TopValue, 1, 0, 0, 1);
    }
    else
        ProgramFail(Parser, "invalid operation");
}


void ExpressionStackCollapse(struct ParseState *Parser, struct ExpressionStack **StackTop, int Precedence, int *IgnorePrecedence)
{
    int FoundPrecedence = Precedence;
    struct Value *TopValue;
    struct Value *BottomValue;
    struct ExpressionStack *TopStackNode = *StackTop;
    struct ExpressionStack *TopOperatorNode;

    debugf("ExpressionStackCollapse(%d):\n", Precedence);



    while (TopStackNode != 0 && TopStackNode->Next != 0 && FoundPrecedence >= Precedence)
    {

        if (TopStackNode->Order == OrderNone)
            TopOperatorNode = TopStackNode->Next;
        else
            TopOperatorNode = TopStackNode;

        FoundPrecedence = TopOperatorNode->Precedence;


        if (FoundPrecedence >= Precedence && TopOperatorNode != 0)
        {

            switch (TopOperatorNode->Order)
            {
                case OrderPrefix:

                    debugf("prefix evaluation\n");
                    TopValue = TopStackNode->Val;


                    HeapPopStack(Parser->pc, 0, sizeof(struct ExpressionStack) + sizeof(struct Value) + TypeStackSizeValue(TopValue));
                    HeapPopStack(Parser->pc, TopOperatorNode, sizeof(struct ExpressionStack));
                    *StackTop = TopOperatorNode->Next;


                    if (Parser->Mode == RunModeRun )
                    {

                        ExpressionPrefixOperator(Parser, StackTop, TopOperatorNode->Op, TopValue);
                    }
                    else
                    {

                        ExpressionPushInt(Parser, StackTop, 0);
                    }
                    break;

                case OrderPostfix:

                    debugf("postfix evaluation\n");
                    TopValue = TopStackNode->Next->Val;


                    HeapPopStack(Parser->pc, 0, sizeof(struct ExpressionStack));
                    HeapPopStack(Parser->pc, TopValue, sizeof(struct ExpressionStack) + sizeof(struct Value) + TypeStackSizeValue(TopValue));
                    *StackTop = TopStackNode->Next->Next;


                    if (Parser->Mode == RunModeRun )
                    {

                        ExpressionPostfixOperator(Parser, StackTop, TopOperatorNode->Op, TopValue);
                    }
                    else
                    {

                        ExpressionPushInt(Parser, StackTop, 0);
                    }
                    break;

                case OrderInfix:

                    debugf("infix evaluation\n");
                    TopValue = TopStackNode->Val;
                    if (TopValue != 0)
                    {
                        BottomValue = TopOperatorNode->Next->Val;


                        HeapPopStack(Parser->pc, 0, sizeof(struct ExpressionStack) + sizeof(struct Value) + TypeStackSizeValue(TopValue));
                        HeapPopStack(Parser->pc, 0, sizeof(struct ExpressionStack));
                        HeapPopStack(Parser->pc, BottomValue, sizeof(struct ExpressionStack) + sizeof(struct Value) + TypeStackSizeValue(BottomValue));
                        *StackTop = TopOperatorNode->Next->Next;


                        if (Parser->Mode == RunModeRun )
                        {

                            ExpressionInfixOperator(Parser, StackTop, TopOperatorNode->Op, BottomValue, TopValue);
                        }
                        else
                        {

                            ExpressionPushInt(Parser, StackTop, 0);
                        }
                    }
                    else
                        FoundPrecedence = -1;
                    break;

                case OrderNone:

                    assert(TopOperatorNode->Order != OrderNone);
                    break;
            }


            if (FoundPrecedence <= *IgnorePrecedence)
                *IgnorePrecedence = (20*1000);
        }



        TopStackNode = *StackTop;
    }
    debugf("ExpressionStackCollapse() finished\n");



}


void ExpressionStackPushOperator(struct ParseState *Parser, struct ExpressionStack **StackTop, enum OperatorOrder Order, enum LexToken Token, int Precedence)
{
    struct ExpressionStack *StackNode = VariableAlloc(Parser->pc, Parser, sizeof(struct ExpressionStack), 0);
    StackNode->Next = *StackTop;
    StackNode->Order = Order;
    StackNode->Op = Token;
    StackNode->Precedence = Precedence;
    *StackTop = StackNode;
    debugf("ExpressionStackPushOperator()\n");







}


void ExpressionGetStructElement(struct ParseState *Parser, struct ExpressionStack **StackTop, enum LexToken Token)
{
    struct Value *Ident;


    if (LexGetToken(Parser, &Ident, 1) != TokenIdentifier)
        ProgramFail(Parser, "need an structure or union member after '%s'", (Token == TokenDot) ? "." : "->");

    if (Parser->Mode == RunModeRun)
    {

        struct Value *ParamVal = (*StackTop)->Val;
        struct Value *StructVal = ParamVal;
        struct ValueType *StructType = ParamVal->Typ;
        char *DerefDataLoc = (char *)ParamVal->Val;
        struct Value *MemberValue = 0;
        struct Value *Result;


        if (Token == TokenArrow)
            DerefDataLoc = VariableDereferencePointer(Parser, ParamVal, &StructVal, 0, &StructType, 0);

        if (StructType->Base != TypeStruct && StructType->Base != TypeUnion)
            ProgramFail(Parser, "can't use '%s' on something that's not a struct or union %s : it's a %t", (Token == TokenDot) ? "." : "->", (Token == TokenArrow) ? "pointer" : "", ParamVal->Typ);

        if (!TableGet(StructType->Members, Ident->Val->Identifier, &MemberValue, 0, 0, 0))
            ProgramFail(Parser, "doesn't have a member called '%s'", Ident->Val->Identifier);


        HeapPopStack(Parser->pc, ParamVal, sizeof(struct ExpressionStack) + sizeof(struct Value) + TypeStackSizeValue(StructVal));
        *StackTop = (*StackTop)->Next;


        Result = VariableAllocValueFromExistingData(Parser, MemberValue->Typ, (void *)(DerefDataLoc + MemberValue->Val->Integer), 1, (StructVal != 0) ? StructVal->LValueFrom : 0);
        ExpressionStackPushValueNode(Parser, StackTop, Result);
    }
}


int ExpressionParse(struct ParseState *Parser, struct Value **Result)
{
    struct Value *LexValue;
    int PrefixState = 1;
    int Done = 0;
    int BracketPrecedence = 0;
    int LocalPrecedence;
    int Precedence = 0;
    int IgnorePrecedence = (20*1000);
    struct ExpressionStack *StackTop = 0;
    int TernaryDepth = 0;

    debugf("ExpressionParse():\n");
    do
    {
        struct ParseState PreState;
        enum LexToken Token;

        ParserCopy(&PreState, Parser);
        Token = LexGetToken(Parser, &LexValue, 1);
        if ( ( ( (int)Token > TokenComma && (int)Token <= (int)TokenOpenBracket) ||
               (Token == TokenCloseBracket && BracketPrecedence != 0)) &&
               (Token != TokenColon || TernaryDepth > 0) )
        {

            if (PrefixState)
            {

                if (OperatorPrecedence[(int)Token].PrefixPrecedence == 0)
                    ProgramFail(Parser, "operator not expected here");

                LocalPrecedence = OperatorPrecedence[(int)Token].PrefixPrecedence;
                Precedence = BracketPrecedence + LocalPrecedence;

                if (Token == TokenOpenBracket)
                {

                    enum LexToken BracketToken = LexGetToken(Parser, &LexValue, 0);
                    if (IsTypeToken(Parser, BracketToken, LexValue) && (StackTop == 0 || StackTop->Op != TokenSizeof) )
                    {

                        struct ValueType *CastType;
                        char *CastIdentifier;
                        struct Value *CastTypeValue;

                        TypeParse(Parser, &CastType, &CastIdentifier, 0);
                        if (LexGetToken(Parser, &LexValue, 1) != TokenCloseBracket)
                            ProgramFail(Parser, "brackets not closed");


                        Precedence = BracketPrecedence + OperatorPrecedence[(int)TokenCast].PrefixPrecedence;

                        ExpressionStackCollapse(Parser, &StackTop, Precedence+1, &IgnorePrecedence);
                        CastTypeValue = VariableAllocValueFromType(Parser->pc, Parser, &Parser->pc->TypeType, 0, 0, 0);
                        CastTypeValue->Val->Typ = CastType;
                        ExpressionStackPushValueNode(Parser, &StackTop, CastTypeValue);
                        ExpressionStackPushOperator(Parser, &StackTop, OrderInfix, TokenCast, Precedence);
                    }
                    else
                    {

                        BracketPrecedence += 20;
                    }
                }
                else
                {



                    int NextToken = LexGetToken(Parser, 0, 0);
                    int TempPrecedenceBoost = 0;
                    if (NextToken > TokenComma && NextToken < TokenOpenBracket)
                    {
                        int NextPrecedence = OperatorPrecedence[(int)NextToken].PrefixPrecedence;



                        if (LocalPrecedence == NextPrecedence)
                            TempPrecedenceBoost = -1;
                    }

                    ExpressionStackCollapse(Parser, &StackTop, Precedence, &IgnorePrecedence);
                    ExpressionStackPushOperator(Parser, &StackTop, OrderPrefix, Token, Precedence + TempPrecedenceBoost);
                }
            }
            else
            {

                if (OperatorPrecedence[(int)Token].PostfixPrecedence != 0)
                {
                    switch (Token)
                    {
                        case TokenCloseBracket:
                        case TokenRightSquareBracket:
                            if (BracketPrecedence == 0)
                            {

                                ParserCopy(Parser, &PreState);
                                Done = 1;
                            }
                            else
                            {

                                ExpressionStackCollapse(Parser, &StackTop, BracketPrecedence, &IgnorePrecedence);
                                BracketPrecedence -= 20;
                            }
                            break;

                        default:

                            Precedence = BracketPrecedence + OperatorPrecedence[(int)Token].PostfixPrecedence;
                            ExpressionStackCollapse(Parser, &StackTop, Precedence, &IgnorePrecedence);
                            ExpressionStackPushOperator(Parser, &StackTop, OrderPostfix, Token, Precedence);
                            break;
                    }
                }
                else if (OperatorPrecedence[(int)Token].InfixPrecedence != 0)
                {

                    Precedence = BracketPrecedence + OperatorPrecedence[(int)Token].InfixPrecedence;



                    if (((OperatorPrecedence[(int)Token].InfixPrecedence) != 2 && (OperatorPrecedence[(int)Token].InfixPrecedence) != 14))
                        ExpressionStackCollapse(Parser, &StackTop, Precedence, &IgnorePrecedence);
                    else
                        ExpressionStackCollapse(Parser, &StackTop, Precedence+1, &IgnorePrecedence);

                    if (Token == TokenDot || Token == TokenArrow)
                    {
                        ExpressionGetStructElement(Parser, &StackTop, Token);
                    }
                    else
                    {

                        if ( (Token == TokenLogicalOr || Token == TokenLogicalAnd) && ((((StackTop->Val)->Typ)->Base >= TypeInt && ((StackTop->Val)->Typ)->Base <= TypeUnsignedLong) || ((StackTop->Val)->Typ->Base == TypeFP)))
                        {
                            long LHSInt = ExpressionCoerceInteger(StackTop->Val);
                            if ( ( (Token == TokenLogicalOr && LHSInt) || (Token == TokenLogicalAnd && !LHSInt) ) &&
                                 (IgnorePrecedence > Precedence) )
                                IgnorePrecedence = Precedence;
                        }


                        ExpressionStackPushOperator(Parser, &StackTop, OrderInfix, Token, Precedence);
                        PrefixState = 1;

                        switch (Token)
                        {
                            case TokenQuestionMark: TernaryDepth++; break;
                            case TokenColon: TernaryDepth--; break;
                            default: break;
                        }
                    }


                    if (Token == TokenLeftSquareBracket)
                    {

                        BracketPrecedence += 20;
                    }
                }
                else
                    ProgramFail(Parser, "operator not expected here");
            }
        }
        else if (Token == TokenIdentifier)
        {

            if (!PrefixState)
                ProgramFail(Parser, "identifier not expected here");

            if (LexGetToken(Parser, 0, 0) == TokenOpenBracket)
            {
                ExpressionParseFunctionCall(Parser, &StackTop, LexValue->Val->Identifier, Parser->Mode == RunModeRun && Precedence < IgnorePrecedence);
            }
            else
            {
                if (Parser->Mode == RunModeRun )
                {
                    struct Value *VariableValue = 0;

                    VariableGet(Parser->pc, Parser, LexValue->Val->Identifier, &VariableValue);
                    if (VariableValue->Typ->Base == TypeMacro)
                    {

                        struct ParseState MacroParser;
                        struct Value *MacroResult;

                        ParserCopy(&MacroParser, &VariableValue->Val->MacroDef.Body);
                        MacroParser.Mode = Parser->Mode;
                        if (VariableValue->Val->MacroDef.NumParams != 0)
                            ProgramFail(&MacroParser, "macro arguments missing");

                        if (!ExpressionParse(&MacroParser, &MacroResult) || LexGetToken(&MacroParser, 0, 0) != TokenEndOfFunction)
                            ProgramFail(&MacroParser, "expression expected");

                        ExpressionStackPushValueNode(Parser, &StackTop, MacroResult);
                    }
                    else if (VariableValue->Typ == &Parser->pc->VoidType)
                        ProgramFail(Parser, "a void value isn't much use here");
                    else
                        ExpressionStackPushLValue(Parser, &StackTop, VariableValue, 0);
                }
                else
                    ExpressionPushInt(Parser, &StackTop, 0);

            }


            if (Precedence <= IgnorePrecedence)
                IgnorePrecedence = (20*1000);

            PrefixState = 0;
        }
        else if ((int)Token > TokenCloseBracket && (int)Token <= TokenCharacterConstant)
        {

            if (!PrefixState)
                ProgramFail(Parser, "value not expected here");

            PrefixState = 0;
            ExpressionStackPushValue(Parser, &StackTop, LexValue);
        }
        else if (IsTypeToken(Parser, Token, LexValue))
        {

            struct ValueType *Typ;
            char *Identifier;
            struct Value *TypeValue;

            if (!PrefixState)
                ProgramFail(Parser, "type not expected here");

            PrefixState = 0;
            ParserCopy(Parser, &PreState);
            TypeParse(Parser, &Typ, &Identifier, 0);
            TypeValue = VariableAllocValueFromType(Parser->pc, Parser, &Parser->pc->TypeType, 0, 0, 0);
            TypeValue->Val->Typ = Typ;
            ExpressionStackPushValueNode(Parser, &StackTop, TypeValue);
        }
        else
        {

            ParserCopy(Parser, &PreState);
            Done = 1;
        }

    } while (!Done);


    if (BracketPrecedence > 0)
        ProgramFail(Parser, "brackets not closed");


    ExpressionStackCollapse(Parser, &StackTop, 0, &IgnorePrecedence);


    if (StackTop != 0)
    {

        if (Parser->Mode == RunModeRun)
        {
            if (StackTop->Order != OrderNone || StackTop->Next != 0)
                ProgramFail(Parser, "invalid expression");

            *Result = StackTop->Val;
            HeapPopStack(Parser->pc, StackTop, sizeof(struct ExpressionStack));
        }
        else
            HeapPopStack(Parser->pc, StackTop->Val, sizeof(struct ExpressionStack) + sizeof(struct Value) + TypeStackSizeValue(StackTop->Val));
    }

    debugf("ExpressionParse() done\n\n");



    return StackTop != 0;
}



void ExpressionParseMacroCall(struct ParseState *Parser, struct ExpressionStack **StackTop, const char *MacroName, struct MacroDef *MDef)
{
    struct Value *ReturnValue = 0;
    struct Value *Param;
    struct Value **ParamArray = 0;
    int ArgCount;
    enum LexToken Token;

    if (Parser->Mode == RunModeRun)
    {


        //ExpressionStackPushValueByType(Parser, StackTop, &Parser->pc->FPType);



        ReturnValue = (*StackTop)->Val;
        HeapPushStackFrame(Parser->pc);
        ParamArray = HeapAllocStack(Parser->pc, sizeof(struct Value *) * MDef->NumParams);
        if (ParamArray == 0)
            ProgramFail(Parser, "out of memory");
    }
    else
        ExpressionPushInt(Parser, StackTop, 0);


    ArgCount = 0;
    do {
        if (ExpressionParse(Parser, &Param))
        {
            if (Parser->Mode == RunModeRun)
            {
                if (ArgCount < MDef->NumParams)
                    ParamArray[ArgCount] = Param;
                else
                    ProgramFail(Parser, "too many arguments to %s()", MacroName);
            }

            ArgCount++;
            Token = LexGetToken(Parser, 0, 1);
            if (Token != TokenComma && Token != TokenCloseBracket)
                ProgramFail(Parser, "comma expected");
        }
        else
        {

            Token = LexGetToken(Parser, 0, 1);
            if (!TokenCloseBracket)
                ProgramFail(Parser, "bad argument");
        }

    } while (Token != TokenCloseBracket);

    if (Parser->Mode == RunModeRun)
    {

        struct ParseState MacroParser;
        int Count;
        struct Value *EvalValue;

        if (ArgCount < MDef->NumParams)
            ProgramFail(Parser, "not enough arguments to '%s'", MacroName);

        if (MDef->Body.Pos == 0)
            ProgramFail(Parser, "'%s' is undefined", MacroName);

        ParserCopy(&MacroParser, &MDef->Body);
        MacroParser.Mode = Parser->Mode;
        VariableStackFrameAdd(Parser, MacroName, 0);
        Parser->pc->TopStackFrame->NumParams = ArgCount;
        Parser->pc->TopStackFrame->ReturnValue = ReturnValue;
        for (Count = 0; Count < MDef->NumParams; Count++)
            VariableDefine(Parser->pc, Parser, MDef->ParamName[Count], ParamArray[Count], 0, 1);

        ExpressionParse(&MacroParser, &EvalValue);
        ExpressionAssign(Parser, ReturnValue, EvalValue, 1, MacroName, 0, 0);
        VariableStackFramePop(Parser);
        HeapPopStackFrame(Parser->pc);
    }
}


void ExpressionParseFunctionCall(struct ParseState *Parser, struct ExpressionStack **StackTop, const char *FuncName, int RunIt)
{
    struct Value *ReturnValue = 0;
    struct Value *FuncValue = 0;
    struct Value *Param;
    struct Value **ParamArray = 0;
    int ArgCount;
    enum LexToken Token = LexGetToken(Parser, 0, 1);
    enum RunMode OldMode = Parser->Mode;

    if (RunIt)
    {

        VariableGet(Parser->pc, Parser, FuncName, &FuncValue);

        if (FuncValue->Typ->Base == TypeMacro)
        {

            ExpressionParseMacroCall(Parser, StackTop, FuncName, &FuncValue->Val->MacroDef);
            return;
        }

        if (FuncValue->Typ->Base != TypeFunction)
            ProgramFail(Parser, "%t is not a function - can't call", FuncValue->Typ);

        ExpressionStackPushValueByType(Parser, StackTop, FuncValue->Val->FuncDef.ReturnType);
        ReturnValue = (*StackTop)->Val;
        HeapPushStackFrame(Parser->pc);
        ParamArray = HeapAllocStack(Parser->pc, sizeof(struct Value *) * FuncValue->Val->FuncDef.NumParams);
        if (ParamArray == 0)
            ProgramFail(Parser, "out of memory");
    }
    else
    {
        ExpressionPushInt(Parser, StackTop, 0);
        Parser->Mode = RunModeSkip;
    }


    ArgCount = 0;
    do {
        if (RunIt && ArgCount < FuncValue->Val->FuncDef.NumParams)
            ParamArray[ArgCount] = VariableAllocValueFromType(Parser->pc, Parser, FuncValue->Val->FuncDef.ParamType[ArgCount], 0, 0, 0);

        if (ExpressionParse(Parser, &Param))
        {
            if (RunIt)
            {
                if (ArgCount < FuncValue->Val->FuncDef.NumParams)
                {
                    ExpressionAssign(Parser, ParamArray[ArgCount], Param, 1, FuncName, ArgCount+1, 0);
                    VariableStackPop(Parser, Param);
                }
                else
                {
                    if (!FuncValue->Val->FuncDef.VarArgs)
                        ProgramFail(Parser, "too many arguments to %s()", FuncName);
                }
            }

            ArgCount++;
            Token = LexGetToken(Parser, 0, 1);
            if (Token != TokenComma && Token != TokenCloseBracket)
                ProgramFail(Parser, "comma expected");
        }
        else
        {

            Token = LexGetToken(Parser, 0, 1);
            if (!TokenCloseBracket)
                ProgramFail(Parser, "bad argument");
        }

    } while (Token != TokenCloseBracket);

    if (RunIt)
    {

        if (ArgCount < FuncValue->Val->FuncDef.NumParams)
            ProgramFail(Parser, "not enough arguments to '%s'", FuncName);

        if (FuncValue->Val->FuncDef.Intrinsic == 0)
        {

            struct ParseState FuncParser;
            int Count;
            int OldScopeID = Parser->ScopeID;

            if (FuncValue->Val->FuncDef.Body.Pos == 0)
                ProgramFail(Parser, "'%s' is undefined", FuncName);

            ParserCopy(&FuncParser, &FuncValue->Val->FuncDef.Body);
            VariableStackFrameAdd(Parser, FuncName, FuncValue->Val->FuncDef.Intrinsic ? FuncValue->Val->FuncDef.NumParams : 0);
            Parser->pc->TopStackFrame->NumParams = ArgCount;
            Parser->pc->TopStackFrame->ReturnValue = ReturnValue;


            Parser->ScopeID = -1;

            for (Count = 0; Count < FuncValue->Val->FuncDef.NumParams; Count++)
                VariableDefine(Parser->pc, Parser, FuncValue->Val->FuncDef.ParamName[Count], ParamArray[Count], 0, 1);

            Parser->ScopeID = OldScopeID;

            if (ParseStatement(&FuncParser, 1) != ParseResultOk)
                ProgramFail(&FuncParser, "function body expected");

            if (RunIt)
            {
                if (FuncParser.Mode == RunModeRun && FuncValue->Val->FuncDef.ReturnType != &Parser->pc->VoidType)
                    ProgramFail(&FuncParser, "no value returned from a function returning %t", FuncValue->Val->FuncDef.ReturnType);

                else if (FuncParser.Mode == RunModeGoto)
                    ProgramFail(&FuncParser, "couldn't find goto label '%s'", FuncParser.SearchGotoLabel);
            }

            VariableStackFramePop(Parser);
        }
        else
            FuncValue->Val->FuncDef.Intrinsic(Parser, ReturnValue, ParamArray, ArgCount);

        HeapPopStackFrame(Parser->pc);
    }

    Parser->Mode = OldMode;
}


long ExpressionParseInt(struct ParseState *Parser)
{
    struct Value *Val;
    long Result = 0;

    if (!ExpressionParse(Parser, &Val))
        ProgramFail(Parser, "expression expected");

    if (Parser->Mode == RunModeRun)
    {
        if (!((((Val)->Typ)->Base >= TypeInt && ((Val)->Typ)->Base <= TypeUnsignedLong) || ((Val)->Typ->Base == TypeFP)))
            ProgramFail(Parser, "integer value expected instead of %t", Val->Typ);

        Result = ExpressionCoerceInteger(Val);
        VariableStackPop(Parser, Val);
    }

    return Result;
}
//# 4 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/heap.c" 1
//# 22 "c-demos/gitlab.com-zsaleeba-picoc/heap.c"
void HeapInit(Picoc *pc, int StackOrHeapSize)
{
    int Count;
    int AlignOffset = 0;
//# 40 "c-demos/gitlab.com-zsaleeba-picoc/heap.c"
    pc->HeapBottom = &pc->HeapMemory[(16*1024)];
    pc->StackFrame = &pc->HeapMemory[0];
    pc->HeapStackTop = &pc->HeapMemory[0];



    while (((unsigned long)&pc->HeapMemory[AlignOffset] & (sizeof(char)-1)) != 0)
        AlignOffset++;

    pc->StackFrame = &(pc->HeapMemory)[AlignOffset];
    pc->HeapStackTop = &(pc->HeapMemory)[AlignOffset];
    *(void **)(pc->StackFrame) = 0;
    pc->HeapBottom = &(pc->HeapMemory)[StackOrHeapSize-sizeof(char)+AlignOffset];
    pc->FreeListBig = 0;
    for (Count = 0; Count < 8; Count++)
        pc->FreeListBucket[Count] = 0;
}

void HeapCleanup(Picoc *pc)
{



}



void *HeapAllocStack(Picoc *pc, int Size)
{
    char *NewMem = pc->HeapStackTop;
    char *NewTop = (char *)pc->HeapStackTop + (((Size) + sizeof(char) - 1) & ~(sizeof(char)-1));



    if (NewTop > (char *)pc->HeapBottom)
        return 0;

    pc->HeapStackTop = (void *)NewTop;
    memset((void *)NewMem, '\0', Size);
    return NewMem;
}


void HeapUnpopStack(Picoc *pc, int Size)
{



    pc->HeapStackTop = (void *)((char *)pc->HeapStackTop + (((Size) + sizeof(char) - 1) & ~(sizeof(char)-1)));
}


int HeapPopStack(Picoc *pc, void *Addr, int Size)
{
    int ToLose = (((Size) + sizeof(char) - 1) & ~(sizeof(char)-1));
    if (ToLose > ((char *)pc->HeapStackTop - (char *)&(pc->HeapMemory)[0]))
        return 0;




    pc->HeapStackTop = (void *)((char *)pc->HeapStackTop - ToLose);
    assert(Addr == 0 || pc->HeapStackTop == Addr);

    return 1;
}


void HeapPushStackFrame(Picoc *pc)
{



    *(void **)pc->HeapStackTop = pc->StackFrame;
    pc->StackFrame = pc->HeapStackTop;
    pc->HeapStackTop = (void *)((char *)pc->HeapStackTop + (((sizeof(char)) + sizeof(char) - 1) & ~(sizeof(char)-1)));
}


int HeapPopStackFrame(Picoc *pc)
{
    if (*(void **)pc->StackFrame != 0)
    {
        pc->HeapStackTop = pc->StackFrame;
        pc->StackFrame = *(void **)pc->StackFrame;



        return 1;
    }
    else
        return 0;
}


void *HeapAllocMem(Picoc *pc, int Size)
{



    struct AllocNode *NewMem = 0;
    struct AllocNode **FreeNode;
    int AllocSize = (((Size) + sizeof(char) - 1) & ~(sizeof(char)-1)) + (((sizeof(NewMem->Size)) + sizeof(char) - 1) & ~(sizeof(char)-1));
    int Bucket;
    void *ReturnMem;

    if (Size == 0)
        return 0;

    assert(Size > 0);


    if (AllocSize < sizeof(struct AllocNode))
        AllocSize = sizeof(struct AllocNode);

    Bucket = AllocSize >> 2;
    if (Bucket < 8 && pc->FreeListBucket[Bucket] != 0)
    {




        NewMem = pc->FreeListBucket[Bucket];
        assert((unsigned long)NewMem >= (unsigned long)&(pc->HeapMemory)[0] && (unsigned char *)NewMem - &(pc->HeapMemory)[0] < (16*1024));
        pc->FreeListBucket[Bucket] = *(struct AllocNode **)NewMem;
        assert(pc->FreeListBucket[Bucket] == 0 || ((unsigned long)pc->FreeListBucket[Bucket] >= (unsigned long)&(pc->HeapMemory)[0] && (unsigned char *)pc->FreeListBucket[Bucket] - &(pc->HeapMemory)[0] < (16*1024)));
        NewMem->Size = AllocSize;
    }
    else if (pc->FreeListBig != 0)
    {

        for (FreeNode = &pc->FreeListBig; *FreeNode != 0 && (*FreeNode)->Size < AllocSize; FreeNode = &(*FreeNode)->NextFree)
        {}

        if (*FreeNode != 0)
        {
            assert((unsigned long)*FreeNode >= (unsigned long)&(pc->HeapMemory)[0] && (unsigned char *)*FreeNode - &(pc->HeapMemory)[0] < (16*1024));
            assert((*FreeNode)->Size < (16*1024) && (*FreeNode)->Size > 0);
            if ((*FreeNode)->Size < AllocSize + 16)
            {




                NewMem = *FreeNode;
                assert((unsigned long)NewMem >= (unsigned long)&(pc->HeapMemory)[0] && (unsigned char *)NewMem - &(pc->HeapMemory)[0] < (16*1024));
                *FreeNode = NewMem->NextFree;
            }
            else
            {




                NewMem = (void *)((char *)*FreeNode + (*FreeNode)->Size - AllocSize);
                assert((unsigned long)NewMem >= (unsigned long)&(pc->HeapMemory)[0] && (unsigned char *)NewMem - &(pc->HeapMemory)[0] < (16*1024));
                (*FreeNode)->Size -= AllocSize;
                NewMem->Size = AllocSize;
            }
        }
    }

    if (NewMem == 0)
    {




        if ((char *)pc->HeapBottom - AllocSize < (char *)pc->HeapStackTop)
            return 0;

        pc->HeapBottom = (void *)((char *)pc->HeapBottom - AllocSize);
        NewMem = pc->HeapBottom;
        NewMem->Size = AllocSize;
    }

    ReturnMem = (void *)((char *)NewMem + (((sizeof(NewMem->Size)) + sizeof(char) - 1) & ~(sizeof(char)-1)));
    memset(ReturnMem, '\0', AllocSize - (((sizeof(NewMem->Size)) + sizeof(char) - 1) & ~(sizeof(char)-1)));



    return ReturnMem;

}


void HeapFreeMem(Picoc *pc, void *Mem)
{



    struct AllocNode *MemNode = (struct AllocNode *)((char *)Mem - (((sizeof(MemNode->Size)) + sizeof(char) - 1) & ~(sizeof(char)-1)));
    int Bucket = MemNode->Size >> 2;




    assert((unsigned long)Mem >= (unsigned long)&(pc->HeapMemory)[0] && (unsigned char *)Mem - &(pc->HeapMemory)[0] < (16*1024));
    assert(MemNode->Size < (16*1024) && MemNode->Size > 0);
    if (Mem == 0)
        return;

    if ((void *)MemNode == pc->HeapBottom)
    {




        pc->HeapBottom = (void *)((char *)pc->HeapBottom + MemNode->Size);



    }
    else if (Bucket < 8)
    {




        assert(pc->FreeListBucket[Bucket] == 0 || ((unsigned long)pc->FreeListBucket[Bucket] >= (unsigned long)&(pc->HeapMemory)[0] && (unsigned char *)pc->FreeListBucket[Bucket] - &pc->HeapMemory[0] < (16*1024)));
        *(struct AllocNode **)MemNode = pc->FreeListBucket[Bucket];
        pc->FreeListBucket[Bucket] = (struct AllocNode *)MemNode;
    }
    else
    {




        assert(pc->FreeListBig == 0 || ((unsigned long)pc->FreeListBig >= (unsigned long)&(pc->HeapMemory)[0] && (unsigned char *)pc->FreeListBig - &(pc->HeapMemory)[0] < (16*1024)));
        MemNode->NextFree = pc->FreeListBig;
        pc->FreeListBig = MemNode;



    }

}
//# 5 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/include.c" 1
//# 6 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/lex.c" 1
//# 31 "c-demos/gitlab.com-zsaleeba-picoc/lex.c"
struct ReservedWord
{
    const char *Word;
    enum LexToken Token;
};

static struct ReservedWord ReservedWords[] =
{
    { "#define", TokenHashDefine },
    { "#else", TokenHashElse },
    { "#endif", TokenHashEndif },
    { "#if", TokenHashIf },
    { "#ifdef", TokenHashIfdef },
    { "#ifndef", TokenHashIfndef },
    { "#include", TokenHashInclude },
    { "auto", TokenAutoType },
    { "break", TokenBreak },
    { "case", TokenCase },
    { "char", TokenCharType },
    { "continue", TokenContinue },
    { "default", TokenDefault },
    { "delete", TokenDelete },
    { "do", TokenDo },

    { "double", TokenDoubleType },

    { "else", TokenElse },
    { "enum", TokenEnumType },
    { "extern", TokenExternType },

    { "float", TokenFloatType },

    { "for", TokenFor },
    { "goto", TokenGoto },
    { "if", TokenIf },
    { "int", TokenIntType },
    { "long", TokenLongType },
    { "new", TokenNew },
    { "register", TokenRegisterType },
    { "return", TokenReturn },
    { "short", TokenShortType },
    { "signed", TokenSignedType },
    { "sizeof", TokenSizeof },
    { "static", TokenStaticType },
    { "struct", TokenStructType },
    { "switch", TokenSwitch },
    { "typedef", TokenTypedef },
    { "union", TokenUnionType },
    { "unsigned", TokenUnsignedType },
    { "void", TokenVoidType },
    { "while", TokenWhile }
};




void LexInit(Picoc *pc)
{
    int Count;

    TableInitTable(&pc->ReservedWordTable, &pc->ReservedWordHashTable[0], sizeof(ReservedWords) / sizeof(struct ReservedWord) * 2, 1);

    for (Count = 0; Count < sizeof(ReservedWords) / sizeof(struct ReservedWord); Count++)
    {
        TableSet(pc, &pc->ReservedWordTable, TableStrRegister(pc, ReservedWords[Count].Word), (struct Value *)&ReservedWords[Count], 0, 0, 0);
    }

    pc->LexValue.Typ = 0;
    pc->LexValue.Val = &pc->LexAnyValue;
    pc->LexValue.LValueFrom = 0;
    pc->LexValue.ValOnHeap = 0;
    pc->LexValue.ValOnStack = 0;
    pc->LexValue.AnyValOnHeap = 0;
    pc->LexValue.IsLValue = 0;
}


void LexCleanup(Picoc *pc)
{
    int Count;

    LexInteractiveClear(pc, 0);

    for (Count = 0; Count < sizeof(ReservedWords) / sizeof(struct ReservedWord); Count++)
        TableDelete(pc, &pc->ReservedWordTable, TableStrRegister(pc, ReservedWords[Count].Word));
}


enum LexToken LexCheckReservedWord(Picoc *pc, const char *Word)
{
    struct Value *val;

    if (TableGet(&pc->ReservedWordTable, Word, &val, 0, 0, 0))
        return ((struct ReservedWord *)val)->Token;
    else
        return TokenNone;
}


enum LexToken LexGetNumber(Picoc *pc, struct LexState *Lexer, struct Value *V)
{
    long Result = 0;
    long Base = 10;
    enum LexToken ResultToken;

    // double FPResult;
    // double FPDiv;







    if (*Lexer->Pos == '0')
    {

        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
        if (Lexer->Pos != Lexer->End)
        {
            if (*Lexer->Pos == 'x' || *Lexer->Pos == 'X')
                { Base = 16; ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); }
            else if (*Lexer->Pos == 'b' || *Lexer->Pos == 'B')
                { Base = 2; ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); }
            else if (*Lexer->Pos != '.')
                Base = 8;
        }
    }


    for (; Lexer->Pos != Lexer->End && (((*Lexer->Pos) >= '0' && (*Lexer->Pos) < '0' + (((Base)<10)?(Base):10)) || (((Base) > 10) ? (((*Lexer->Pos) >= 'a' && (*Lexer->Pos) <= 'f') || ((*Lexer->Pos) >= 'A' && (*Lexer->Pos) <= 'F')) : 0)); ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ))
        Result = Result * Base + (((*Lexer->Pos) <= '9') ? ((*Lexer->Pos) - '0') : (((*Lexer->Pos) <= 'F') ? ((*Lexer->Pos) - 'A' + 10) : ((*Lexer->Pos) - 'a' + 10)));

    if (*Lexer->Pos == 'u' || *Lexer->Pos == 'U')
    {
        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );

    }
    if (*Lexer->Pos == 'l' || *Lexer->Pos == 'L')
    {
        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );

    }

    V->Typ = &pc->LongType;
    V->Val->LongInteger = Result;

    ResultToken = TokenIntegerConstant;

    if (Lexer->Pos == Lexer->End)
        return ResultToken;


    if (Lexer->Pos == Lexer->End)
    {
        return ResultToken;
    }

    if (*Lexer->Pos != '.' && *Lexer->Pos != 'e' && *Lexer->Pos != 'E')
    {
        return ResultToken;
    }

#if 0
    V->Typ = &pc->FPType;
    FPResult = (double)Result;

    if (*Lexer->Pos == '.')
    {
        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
        for (FPDiv = 1.0/Base; Lexer->Pos != Lexer->End && (((*Lexer->Pos) >= '0' && (*Lexer->Pos) < '0' + (((Base)<10)?(Base):10)) || (((Base) > 10) ? (((*Lexer->Pos) >= 'a' && (*Lexer->Pos) <= 'f') || ((*Lexer->Pos) >= 'A' && (*Lexer->Pos) <= 'F')) : 0)); ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ), FPDiv /= (double)Base)
        {
            FPResult += (((*Lexer->Pos) <= '9') ? ((*Lexer->Pos) - '0') : (((*Lexer->Pos) <= 'F') ? ((*Lexer->Pos) - 'A' + 10) : ((*Lexer->Pos) - 'a' + 10))) * FPDiv;
        }
    }

    if (Lexer->Pos != Lexer->End && (*Lexer->Pos == 'e' || *Lexer->Pos == 'E'))
    {
        int ExponentSign = 1;

        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
        if (Lexer->Pos != Lexer->End && *Lexer->Pos == '-')
        {
            ExponentSign = -1;
            ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
        }

        Result = 0;
        while (Lexer->Pos != Lexer->End && (((*Lexer->Pos) >= '0' && (*Lexer->Pos) < '0' + (((Base)<10)?(Base):10)) || (((Base) > 10) ? (((*Lexer->Pos) >= 'a' && (*Lexer->Pos) <= 'f') || ((*Lexer->Pos) >= 'A' && (*Lexer->Pos) <= 'F')) : 0)))
        {
            Result = Result * Base + (((*Lexer->Pos) <= '9') ? ((*Lexer->Pos) - '0') : (((*Lexer->Pos) <= 'F') ? ((*Lexer->Pos) - 'A' + 10) : ((*Lexer->Pos) - 'a' + 10)));
            ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
        }

        FPResult *= pow((double)Base, (double)Result * ExponentSign);
    }

    V->Val->FP = FPResult;

    if (*Lexer->Pos == 'f' || *Lexer->Pos == 'F')
        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );

#endif
    abort();
    return TokenFPConstant;


}


enum LexToken LexGetWord(Picoc *pc, struct LexState *Lexer, struct Value *V)
{
    const char *StartPos = Lexer->Pos;
    enum LexToken Token;

    do {
        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
    } while (Lexer->Pos != Lexer->End && (isalnum((int)*Lexer->Pos) || ((int)*Lexer->Pos)=='_'));

    V->Typ = 0;
    V->Val->Identifier = TableStrRegister2(pc, StartPos, Lexer->Pos - StartPos);

    Token = LexCheckReservedWord(pc, V->Val->Identifier);
    switch (Token)
    {
        case TokenHashInclude: Lexer->Mode = LexModeHashInclude; break;
        case TokenHashDefine: Lexer->Mode = LexModeHashDefine; break;
        default: break;
    }

    if (Token != TokenNone)
        return Token;

    if (Lexer->Mode == LexModeHashDefineSpace)
        Lexer->Mode = LexModeHashDefineSpaceIdent;

    return TokenIdentifier;
}


unsigned char LexUnEscapeCharacterConstant(const char **From, const char *End, unsigned char FirstChar, int Base)
{
    unsigned char Total = (((FirstChar) <= '9') ? ((FirstChar) - '0') : (((FirstChar) <= 'F') ? ((FirstChar) - 'A' + 10) : ((FirstChar) - 'a' + 10)));
    int CCount;
    for (CCount = 0; (((**From) >= '0' && (**From) < '0' + (((Base)<10)?(Base):10)) || (((Base) > 10) ? (((**From) >= 'a' && (**From) <= 'f') || ((**From) >= 'A' && (**From) <= 'F')) : 0)) && CCount < 2; CCount++, (*From)++)
        Total = Total * Base + (((**From) <= '9') ? ((**From) - '0') : (((**From) <= 'F') ? ((**From) - 'A' + 10) : ((**From) - 'a' + 10)));

    return Total;
}


unsigned char LexUnEscapeCharacter(const char **From, const char *End)
{
    unsigned char ThisChar;

    while ( *From != End && **From == '\\' &&
            &(*From)[1] != End && (*From)[1] == '\n' )
        (*From) += 2;

    while ( *From != End && **From == '\\' &&
            &(*From)[1] != End && &(*From)[2] != End && (*From)[1] == '\r' && (*From)[2] == '\n')
        (*From) += 3;

    if (*From == End)
        return '\\';

    if (**From == '\\')
    {

        (*From)++;
        if (*From == End)
            return '\\';

        ThisChar = *(*From)++;
        switch (ThisChar)
        {
            case '\\': return '\\';
            case '\'': return '\'';
            case '"': return '"';
            case 'a': return '\a';
            case 'b': return '\b';
            case 'f': return '\f';
            case 'n': return '\n';
            case 'r': return '\r';
            case 't': return '\t';
            case 'v': return '\v';
            case '0': case '1': case '2': case '3': return LexUnEscapeCharacterConstant(From, End, ThisChar, 8);
            case 'x': return LexUnEscapeCharacterConstant(From, End, '0', 16);
            default: return ThisChar;
        }
    }
    else
        return *(*From)++;
}


enum LexToken LexGetStringConstant(Picoc *pc, struct LexState *Lexer, struct Value *V, char EndChar)
{
    int Escape = 0;
    const char *StartPos = Lexer->Pos;
    const char *EndPos;
    char *EscBuf;
    char *EscBufPos;
    char *RegString;
    struct Value *ArrayValue;

    while (Lexer->Pos != Lexer->End && (*Lexer->Pos != EndChar || Escape))
    {

        if (Escape)
        {
            if (*Lexer->Pos == '\r' && Lexer->Pos+1 != Lexer->End)
                Lexer->Pos++;

            if (*Lexer->Pos == '\n' && Lexer->Pos+1 != Lexer->End)
            {
                Lexer->Line++;
                Lexer->Pos++;
                Lexer->CharacterPos = 0;
                Lexer->EmitExtraNewlines++;
            }

            Escape = 0;
        }
        else if (*Lexer->Pos == '\\')
            Escape = 1;

        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
    }
    EndPos = Lexer->Pos;

    EscBuf = HeapAllocStack(pc, EndPos - StartPos);
    if (EscBuf == 0)
        LexFail(pc, Lexer, "out of memory");

    for (EscBufPos = EscBuf, Lexer->Pos = StartPos; Lexer->Pos != EndPos;)
        *EscBufPos++ = LexUnEscapeCharacter(&Lexer->Pos, EndPos);


    RegString = TableStrRegister2(pc, EscBuf, EscBufPos - EscBuf);
    HeapPopStack(pc, EscBuf, EndPos - StartPos);
    ArrayValue = VariableStringLiteralGet(pc, RegString);
    if (ArrayValue == 0)
    {

        ArrayValue = VariableAllocValueAndData(pc, 0, 0, 0, 0, 1);
        ArrayValue->Typ = pc->CharArrayType;
        ArrayValue->Val = (union AnyValue *)RegString;
        VariableStringLiteralDefine(pc, RegString, ArrayValue);
    }


    V->Typ = pc->CharPtrType;
    V->Val->Pointer = RegString;
    if (*Lexer->Pos == EndChar)
        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );

    return TokenStringConstant;
}


enum LexToken LexGetCharacterConstant(Picoc *pc, struct LexState *Lexer, struct Value *V)
{
    V->Typ = &pc->CharType;
    V->Val->Character = LexUnEscapeCharacter(&Lexer->Pos, Lexer->End);
    if (Lexer->Pos != Lexer->End && *Lexer->Pos != '\'')
        LexFail(pc, Lexer, "expected \"'\"");

    ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
    return TokenCharacterConstant;
}


void LexSkipComment(struct LexState *Lexer, char NextChar, enum LexToken *ReturnToken)
{
    if (NextChar == '*')
    {

        while (Lexer->Pos != Lexer->End && (*(Lexer->Pos-1) != '*' || *Lexer->Pos != '/'))
        {
            if (*Lexer->Pos == '\n')
                Lexer->EmitExtraNewlines++;

            ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
        }

        if (Lexer->Pos != Lexer->End)
            ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );

        Lexer->Mode = LexModeNormal;
    }
    else
    {

        while (Lexer->Pos != Lexer->End && *Lexer->Pos != '\n')
            ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
    }
}


enum LexToken LexScanGetToken(Picoc *pc, struct LexState *Lexer, struct Value **Val)
{
    char ThisChar;
    char NextChar;
    enum LexToken GotToken = TokenNone;


    if (Lexer->EmitExtraNewlines > 0)
    {
        Lexer->EmitExtraNewlines--;
        return TokenEndOfLine;
    }


    do
    {
        *Val = &pc->LexValue;
        while (Lexer->Pos != Lexer->End && isspace((int)*Lexer->Pos))
        {
            if (*Lexer->Pos == '\n')
            {
                Lexer->Line++;
                Lexer->Pos++;
                Lexer->Mode = LexModeNormal;
                Lexer->CharacterPos = 0;
                return TokenEndOfLine;
            }
            else if (Lexer->Mode == LexModeHashDefine || Lexer->Mode == LexModeHashDefineSpace)
                Lexer->Mode = LexModeHashDefineSpace;

            else if (Lexer->Mode == LexModeHashDefineSpaceIdent)
                Lexer->Mode = LexModeNormal;

            ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
        }

        if (Lexer->Pos == Lexer->End || *Lexer->Pos == '\0')
            return TokenEOF;

        ThisChar = *Lexer->Pos;
        if ((isalpha((int)ThisChar) || ((int)ThisChar)=='_' || ((int)ThisChar)=='#'))
            return LexGetWord(pc, Lexer, *Val);

        if (isdigit((int)ThisChar))
            return LexGetNumber(pc, Lexer, *Val);

        NextChar = (Lexer->Pos+1 != Lexer->End) ? *(Lexer->Pos+1) : 0;
        ( (Lexer)->Pos++, (Lexer)->CharacterPos++ );
        switch (ThisChar)
        {
            case '"': GotToken = LexGetStringConstant(pc, Lexer, *Val, '"'); break;
            case '\'': GotToken = LexGetCharacterConstant(pc, Lexer, *Val); break;
            case '(': if (Lexer->Mode == LexModeHashDefineSpaceIdent) GotToken = TokenOpenMacroBracket; else GotToken = TokenOpenBracket; Lexer->Mode = LexModeNormal; break;
            case ')': GotToken = TokenCloseBracket; break;
            case '=': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenEqual); } else GotToken = (TokenAssign); }; break;
            case '+': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenAddAssign); } else { if (NextChar == ('+')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenIncrement); } else GotToken = (TokenPlus); } }; break;
            case '-': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenSubtractAssign); } else { if (NextChar == ('>')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenArrow); } else { if (NextChar == ('-')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenDecrement); } else GotToken = (TokenMinus); } } }; break;
            case '*': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenMultiplyAssign); } else GotToken = (TokenAsterisk); }; break;
            case '/': if (NextChar == '/' || NextChar == '*') { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); LexSkipComment(Lexer, NextChar, &GotToken); } else { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenDivideAssign); } else GotToken = (TokenSlash); }; break;
            case '%': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenModulusAssign); } else GotToken = (TokenModulus); }; break;
            case '<': if (Lexer->Mode == LexModeHashInclude) GotToken = LexGetStringConstant(pc, Lexer, *Val, '>'); else { { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenLessEqual); } else if (NextChar == ('<')) { if (Lexer->Pos[1] == ('=')) { ( (Lexer)->Pos+=(2), (Lexer)->CharacterPos+=(2) ); GotToken = (TokenShiftLeftAssign); } else { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenShiftLeft); } } else GotToken = (TokenLessThan); }; } break;
            case '>': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenGreaterEqual); } else if (NextChar == ('>')) { if (Lexer->Pos[1] == ('=')) { ( (Lexer)->Pos+=(2), (Lexer)->CharacterPos+=(2) ); GotToken = (TokenShiftRightAssign); } else { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenShiftRight); } } else GotToken = (TokenGreaterThan); }; break;
            case ';': GotToken = TokenSemicolon; break;
            case '&': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenArithmeticAndAssign); } else { if (NextChar == ('&')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenLogicalAnd); } else GotToken = (TokenAmpersand); } }; break;
            case '|': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenArithmeticOrAssign); } else { if (NextChar == ('|')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenLogicalOr); } else GotToken = (TokenArithmeticOr); } }; break;
            case '{': GotToken = TokenLeftBrace; break;
            case '}': GotToken = TokenRightBrace; break;
            case '[': GotToken = TokenLeftSquareBracket; break;
            case ']': GotToken = TokenRightSquareBracket; break;
            case '!': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenNotEqual); } else GotToken = (TokenUnaryNot); }; break;
            case '^': { if (NextChar == ('=')) { ( (Lexer)->Pos++, (Lexer)->CharacterPos++ ); GotToken = (TokenArithmeticExorAssign); } else GotToken = (TokenArithmeticExor); }; break;
            case '~': GotToken = TokenUnaryExor; break;
            case ',': GotToken = TokenComma; break;
            case '.': { if (NextChar == ('.') && Lexer->Pos[1] == ('.')) { ( (Lexer)->Pos+=(2), (Lexer)->CharacterPos+=(2) ); GotToken = (TokenEllipsis); } else GotToken = (TokenDot); }; break;
            case '?': GotToken = TokenQuestionMark; break;
            case ':': GotToken = TokenColon; break;
            default: LexFail(pc, Lexer, "illegal character '%c'", ThisChar); break;
        }
    } while (GotToken == TokenNone);

    return GotToken;
}


int LexTokenSize(enum LexToken Token)
{
    switch (Token)
    {
        case TokenIdentifier: case TokenStringConstant: return sizeof(char *);
        case TokenIntegerConstant: return sizeof(long);
        case TokenCharacterConstant: return sizeof(unsigned char);
        //case TokenFPConstant: return sizeof(double);
        default: return 0;
    }
}


void *LexTokenise(Picoc *pc, struct LexState *Lexer, int *TokenLen)
{
    enum LexToken Token;
    void *HeapMem;
    struct Value *GotValue;
    int MemUsed = 0;
    int ValueSize;
    int ReserveSpace = (Lexer->End - Lexer->Pos) * 4 + 16;
    void *TokenSpace = HeapAllocStack(pc, ReserveSpace);
    char *TokenPos = (char *)TokenSpace;
    int LastCharacterPos = 0;

    if (TokenSpace == 0)
        LexFail(pc, Lexer, "out of memory");

    do
    {

        Token = LexScanGetToken(pc, Lexer, &GotValue);




        *(unsigned char *)TokenPos = Token;
        TokenPos++;
        MemUsed++;

        *(unsigned char *)TokenPos = (unsigned char)LastCharacterPos;
        TokenPos++;
        MemUsed++;

        ValueSize = LexTokenSize(Token);
        if (ValueSize > 0)
        {

            memcpy((void *)TokenPos, (void *)GotValue->Val, ValueSize);
            TokenPos += ValueSize;
            MemUsed += ValueSize;
        }

        LastCharacterPos = Lexer->CharacterPos;

    } while (Token != TokenEOF);

    HeapMem = HeapAllocMem(pc, MemUsed);
    if (HeapMem == 0)
        LexFail(pc, Lexer, "out of memory");

    assert(ReserveSpace >= MemUsed);
    memcpy(HeapMem, TokenSpace, MemUsed);
    HeapPopStack(pc, TokenSpace, ReserveSpace);
//# 586 "c-demos/gitlab.com-zsaleeba-picoc/lex.c"
    if (TokenLen)
        *TokenLen = MemUsed;

    return HeapMem;
}


void *LexAnalyse(Picoc *pc, const char *FileName, const char *Source, int SourceLen, int *TokenLen)
{
    struct LexState Lexer;

    Lexer.Pos = Source;
    Lexer.End = Source + SourceLen;
    Lexer.Line = 1;
    Lexer.FileName = FileName;
    Lexer.Mode = LexModeNormal;
    Lexer.EmitExtraNewlines = 0;
    Lexer.CharacterPos = 1;
    Lexer.SourceText = Source;

    return LexTokenise(pc, &Lexer, TokenLen);
}


void LexInitParser(struct ParseState *Parser, Picoc *pc, const char *SourceText, void *TokenSource, char *FileName, int RunIt, int EnableDebugger)
{
    Parser->pc = pc;
    Parser->Pos = TokenSource;
    Parser->Line = 1;
    Parser->FileName = FileName;
    Parser->Mode = RunIt ? RunModeRun : RunModeSkip;
    Parser->SearchLabel = 0;
    Parser->HashIfLevel = 0;
    Parser->HashIfEvaluateToLevel = 0;
    Parser->CharacterPos = 0;
    Parser->SourceText = SourceText;
    Parser->DebugMode = EnableDebugger;
}


enum LexToken LexGetRawToken(struct ParseState *Parser, struct Value **Val, int IncPos)
{
    enum LexToken Token = TokenNone;
    int ValueSize;
    char *Prompt = 0;
    Picoc *pc = Parser->pc;

    do
    {

        if (Parser->Pos == 0 && pc->InteractiveHead != 0)
            Parser->Pos = pc->InteractiveHead->Tokens;

        if (Parser->FileName != pc->StrEmpty || pc->InteractiveHead != 0)
        {

            while ((Token = (enum LexToken)*(unsigned char *)Parser->Pos) == TokenEndOfLine)
            {
                Parser->Line++;
                Parser->Pos += 2;
            }
        }

        if (Parser->FileName == pc->StrEmpty && (pc->InteractiveHead == 0 || Token == TokenEOF))
        {

            char LineBuffer[256];
            void *LineTokens;
            int LineBytes;
            struct TokenLine *LineNode;

            if (pc->InteractiveHead == 0 || (unsigned char *)Parser->Pos == &pc->InteractiveTail->Tokens[pc->InteractiveTail->NumBytes-2])
            {

                if (pc->LexUseStatementPrompt)
                {
                    Prompt = "picoc> ";
                    pc->LexUseStatementPrompt = 0;
                }
                else
                    Prompt = "     > ";

                if (PlatformGetLine(&LineBuffer[0], 256, Prompt) == 0)
                    return TokenEOF;


                LineTokens = LexAnalyse(pc, pc->StrEmpty, &LineBuffer[0], strlen(LineBuffer), &LineBytes);
                LineNode = VariableAlloc(pc, Parser, sizeof(struct TokenLine), 1);
                LineNode->Tokens = LineTokens;
                LineNode->NumBytes = LineBytes;
                if (pc->InteractiveHead == 0)
                {

                    pc->InteractiveHead = LineNode;
                    Parser->Line = 1;
                    Parser->CharacterPos = 0;
                }
                else
                    pc->InteractiveTail->Next = LineNode;

                pc->InteractiveTail = LineNode;
                pc->InteractiveCurrentLine = LineNode;
                Parser->Pos = LineTokens;
            }
            else
            {

                if (Parser->Pos != &pc->InteractiveCurrentLine->Tokens[pc->InteractiveCurrentLine->NumBytes-2])
                {

                    for (pc->InteractiveCurrentLine = pc->InteractiveHead; Parser->Pos != &pc->InteractiveCurrentLine->Tokens[pc->InteractiveCurrentLine->NumBytes-2]; pc->InteractiveCurrentLine = pc->InteractiveCurrentLine->Next)
                    { assert(pc->InteractiveCurrentLine->Next != 0); }
                }

                assert(pc->InteractiveCurrentLine != 0);
                pc->InteractiveCurrentLine = pc->InteractiveCurrentLine->Next;
                assert(pc->InteractiveCurrentLine != 0);
                Parser->Pos = pc->InteractiveCurrentLine->Tokens;
            }

            Token = (enum LexToken)*(unsigned char *)Parser->Pos;
        }
    } while ((Parser->FileName == pc->StrEmpty && Token == TokenEOF) || Token == TokenEndOfLine);

    Parser->CharacterPos = *((unsigned char *)Parser->Pos + 1);
    ValueSize = LexTokenSize(Token);
    if (ValueSize > 0)
    {

        if (Val != 0)
        {
            switch (Token)
            {
                case TokenStringConstant: pc->LexValue.Typ = pc->CharPtrType; break;
                case TokenIdentifier: pc->LexValue.Typ = 0; break;
                case TokenIntegerConstant: pc->LexValue.Typ = &pc->LongType; break;
                case TokenCharacterConstant: pc->LexValue.Typ = &pc->CharType; break;

                //case TokenFPConstant: pc->LexValue.Typ = &pc->FPType; break;

                default: break;
            }

            memcpy((void *)pc->LexValue.Val, (void *)((char *)Parser->Pos + 2), ValueSize);
            pc->LexValue.ValOnHeap = 0;
            pc->LexValue.ValOnStack = 0;
            pc->LexValue.IsLValue = 0;
            pc->LexValue.LValueFrom = 0;
            *Val = &pc->LexValue;
        }

        if (IncPos)
            Parser->Pos += ValueSize + 2;
    }
    else
    {
        if (IncPos && Token != TokenEOF)
            Parser->Pos += 2;
    }




    assert(Token >= TokenNone && Token <= TokenEndOfFunction);
    return Token;
}


void LexHashIncPos(struct ParseState *Parser, int IncPos)
{
    if (!IncPos)
        LexGetRawToken(Parser, 0, 1);
}


void LexHashIfdef(struct ParseState *Parser, int IfNot)
{

    struct Value *IdentValue;
    struct Value *SavedValue;
    int IsDefined;
    enum LexToken Token = LexGetRawToken(Parser, &IdentValue, 1);

    if (Token != TokenIdentifier)
        ProgramFail(Parser, "identifier expected");


    IsDefined = TableGet(&Parser->pc->GlobalTable, IdentValue->Val->Identifier, &SavedValue, 0, 0, 0);
    if (Parser->HashIfEvaluateToLevel == Parser->HashIfLevel && ( (IsDefined && !IfNot) || (!IsDefined && IfNot)) )
    {

        Parser->HashIfEvaluateToLevel++;
    }

    Parser->HashIfLevel++;
}


void LexHashIf(struct ParseState *Parser)
{

    struct Value *IdentValue;
    struct Value *SavedValue = 0;
    struct ParseState MacroParser;
    enum LexToken Token = LexGetRawToken(Parser, &IdentValue, 1);

    if (Token == TokenIdentifier)
    {

        if (!TableGet(&Parser->pc->GlobalTable, IdentValue->Val->Identifier, &SavedValue, 0, 0, 0))
            ProgramFail(Parser, "'%s' is undefined", IdentValue->Val->Identifier);

        if (SavedValue->Typ->Base != TypeMacro)
            ProgramFail(Parser, "value expected");

        ParserCopy(&MacroParser, &SavedValue->Val->MacroDef.Body);
        Token = LexGetRawToken(&MacroParser, &IdentValue, 1);
    }

    if (Token != TokenCharacterConstant && Token != TokenIntegerConstant)
        ProgramFail(Parser, "value expected");


    if (Parser->HashIfEvaluateToLevel == Parser->HashIfLevel && IdentValue->Val->Character)
    {

        Parser->HashIfEvaluateToLevel++;
    }

    Parser->HashIfLevel++;
}


void LexHashElse(struct ParseState *Parser)
{
    if (Parser->HashIfEvaluateToLevel == Parser->HashIfLevel - 1)
        Parser->HashIfEvaluateToLevel++;

    else if (Parser->HashIfEvaluateToLevel == Parser->HashIfLevel)
    {

        if (Parser->HashIfLevel == 0)
            ProgramFail(Parser, "#else without #if");

        Parser->HashIfEvaluateToLevel--;
    }
}


void LexHashEndif(struct ParseState *Parser)
{
    if (Parser->HashIfLevel == 0)
        ProgramFail(Parser, "#endif without #if");

    Parser->HashIfLevel--;
    if (Parser->HashIfEvaluateToLevel > Parser->HashIfLevel)
        Parser->HashIfEvaluateToLevel = Parser->HashIfLevel;
}
//# 883 "c-demos/gitlab.com-zsaleeba-picoc/lex.c"
enum LexToken LexGetToken(struct ParseState *Parser, struct Value **Val, int IncPos)
{
    enum LexToken Token;
    int TryNextToken;


    do
    {
        int WasPreProcToken = 1;

        Token = LexGetRawToken(Parser, Val, IncPos);
        switch (Token)
        {
            case TokenHashIfdef: LexHashIncPos(Parser, IncPos); LexHashIfdef(Parser, 0); break;
            case TokenHashIfndef: LexHashIncPos(Parser, IncPos); LexHashIfdef(Parser, 1); break;
            case TokenHashIf: LexHashIncPos(Parser, IncPos); LexHashIf(Parser); break;
            case TokenHashElse: LexHashIncPos(Parser, IncPos); LexHashElse(Parser); break;
            case TokenHashEndif: LexHashIncPos(Parser, IncPos); LexHashEndif(Parser); break;
            default: WasPreProcToken = 0; break;
        }


        TryNextToken = (Parser->HashIfEvaluateToLevel < Parser->HashIfLevel && Token != TokenEOF) || WasPreProcToken;
        if (!IncPos && TryNextToken)
            LexGetRawToken(Parser, 0, 1);

    } while (TryNextToken);

    return Token;
}


enum LexToken LexRawPeekToken(struct ParseState *Parser)
{
    return (enum LexToken)*(unsigned char *)Parser->Pos;
}


void LexToEndOfLine(struct ParseState *Parser)
{
    while (1)
    {
        enum LexToken Token = (enum LexToken)*(unsigned char *)Parser->Pos;
        if (Token == TokenEndOfLine || Token == TokenEOF)
            return;
        else
            LexGetRawToken(Parser, 0, 1);
    }
}


void *LexCopyTokens(struct ParseState *StartParser, struct ParseState *EndParser)
{
    int MemSize = 0;
    int CopySize;
    unsigned char *Pos = (unsigned char *)StartParser->Pos;
    unsigned char *NewTokens;
    unsigned char *NewTokenPos;
    struct TokenLine *ILine;
    Picoc *pc = StartParser->pc;

    if (pc->InteractiveHead == 0)
    {

        MemSize = EndParser->Pos - StartParser->Pos;
        NewTokens = VariableAlloc(pc, StartParser, MemSize + 2, 1);
        memcpy(NewTokens, (void *)StartParser->Pos, MemSize);
    }
    else
    {

        for (pc->InteractiveCurrentLine = pc->InteractiveHead; pc->InteractiveCurrentLine != 0 && (Pos < &pc->InteractiveCurrentLine->Tokens[0] || Pos >= &pc->InteractiveCurrentLine->Tokens[pc->InteractiveCurrentLine->NumBytes]); pc->InteractiveCurrentLine = pc->InteractiveCurrentLine->Next)
        {}

        if (EndParser->Pos >= StartParser->Pos && EndParser->Pos < &pc->InteractiveCurrentLine->Tokens[pc->InteractiveCurrentLine->NumBytes])
        {

            MemSize = EndParser->Pos - StartParser->Pos;
            NewTokens = VariableAlloc(pc, StartParser, MemSize + 2, 1);
            memcpy(NewTokens, (void *)StartParser->Pos, MemSize);
        }
        else
        {

            MemSize = &pc->InteractiveCurrentLine->Tokens[pc->InteractiveCurrentLine->NumBytes-2] - Pos;

            for (ILine = pc->InteractiveCurrentLine->Next; ILine != 0 && (EndParser->Pos < &ILine->Tokens[0] || EndParser->Pos >= &ILine->Tokens[ILine->NumBytes]); ILine = ILine->Next)
                MemSize += ILine->NumBytes - 2;

            assert(ILine != 0);
            MemSize += EndParser->Pos - &ILine->Tokens[0];
            NewTokens = VariableAlloc(pc, StartParser, MemSize + 2, 1);

            CopySize = &pc->InteractiveCurrentLine->Tokens[pc->InteractiveCurrentLine->NumBytes-2] - Pos;
            memcpy(NewTokens, Pos, CopySize);
            NewTokenPos = NewTokens + CopySize;
            for (ILine = pc->InteractiveCurrentLine->Next; ILine != 0 && (EndParser->Pos < &ILine->Tokens[0] || EndParser->Pos >= &ILine->Tokens[ILine->NumBytes]); ILine = ILine->Next)
            {
                memcpy(NewTokenPos, &ILine->Tokens[0], ILine->NumBytes - 2);
                NewTokenPos += ILine->NumBytes-2;
            }
            assert(ILine != 0);
            memcpy(NewTokenPos, &ILine->Tokens[0], EndParser->Pos - &ILine->Tokens[0]);
        }
    }

    NewTokens[MemSize] = (unsigned char)TokenEndOfFunction;

    return NewTokens;
}


void LexInteractiveClear(Picoc *pc, struct ParseState *Parser)
{
    while (pc->InteractiveHead != 0)
    {
        struct TokenLine *NextLine = pc->InteractiveHead->Next;

        HeapFreeMem(pc, pc->InteractiveHead->Tokens);
        HeapFreeMem(pc, pc->InteractiveHead);
        pc->InteractiveHead = NextLine;
    }

    if (Parser != 0)
        Parser->Pos = 0;

    pc->InteractiveTail = 0;
}


void LexInteractiveCompleted(Picoc *pc, struct ParseState *Parser)
{
    while (pc->InteractiveHead != 0 && !(Parser->Pos >= &pc->InteractiveHead->Tokens[0] && Parser->Pos < &pc->InteractiveHead->Tokens[pc->InteractiveHead->NumBytes]))
    {

        struct TokenLine *NextLine = pc->InteractiveHead->Next;

        HeapFreeMem(pc, pc->InteractiveHead->Tokens);
        HeapFreeMem(pc, pc->InteractiveHead);
        pc->InteractiveHead = NextLine;

        if (pc->InteractiveHead == 0)
        {

            Parser->Pos = 0;
            pc->InteractiveTail = 0;
        }
    }
}


void LexInteractiveStatementPrompt(Picoc *pc)
{
    pc->LexUseStatementPrompt = 1;
}
//# 7 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/parse.c" 1






void ParseCleanup(Picoc *pc)
{
    while (pc->CleanupTokenList != 0)
    {
        struct CleanupTokenNode *Next = pc->CleanupTokenList->Next;

        HeapFreeMem(pc, pc->CleanupTokenList->Tokens);
        if (pc->CleanupTokenList->SourceText != 0)
            HeapFreeMem(pc, (void *)pc->CleanupTokenList->SourceText);

        HeapFreeMem(pc, pc->CleanupTokenList);
        pc->CleanupTokenList = Next;
    }
}


enum ParseResult ParseStatementMaybeRun(struct ParseState *Parser, int Condition, int CheckTrailingSemicolon)
{
    if (Parser->Mode != RunModeSkip && !Condition)
    {
        enum RunMode OldMode = Parser->Mode;
        int Result;
        Parser->Mode = RunModeSkip;
        Result = ParseStatement(Parser, CheckTrailingSemicolon);
        Parser->Mode = OldMode;
        return Result;
    }
    else
        return ParseStatement(Parser, CheckTrailingSemicolon);
}


int ParseCountParams(struct ParseState *Parser)
{
    int ParamCount = 0;

    enum LexToken Token = LexGetToken(Parser, 0, 1);
    if (Token != TokenCloseBracket && Token != TokenEOF)
    {

        ParamCount++;
        while ((Token = LexGetToken(Parser, 0, 1)) != TokenCloseBracket && Token != TokenEOF)
        {
            if (Token == TokenComma)
                ParamCount++;
        }
    }

    return ParamCount;
}


struct Value *ParseFunctionDefinition(struct ParseState *Parser, struct ValueType *ReturnType, char *Identifier)
{
    struct ValueType *ParamType;
    char *ParamIdentifier;
    enum LexToken Token = TokenNone;
    struct ParseState ParamParser;
    struct Value *FuncValue;
    struct Value *OldFuncValue;
    struct ParseState FuncBody;
    int ParamCount = 0;
    Picoc *pc = Parser->pc;

    if (pc->TopStackFrame != 0)
        ProgramFail(Parser, "nested function definitions are not allowed");

    LexGetToken(Parser, 0, 1);
    ParserCopy(&ParamParser, Parser);
    ParamCount = ParseCountParams(Parser);
    if (ParamCount > 16)
        ProgramFail(Parser, "too many parameters (%d allowed)", 16);

    FuncValue = VariableAllocValueAndData(pc, Parser, sizeof(struct FuncDef) + sizeof(struct ValueType *) * ParamCount + sizeof(const char *) * ParamCount, 0, 0, 1);
    FuncValue->Typ = &pc->FunctionType;
    FuncValue->Val->FuncDef.ReturnType = ReturnType;
    FuncValue->Val->FuncDef.NumParams = ParamCount;
    FuncValue->Val->FuncDef.VarArgs = 0;
    FuncValue->Val->FuncDef.ParamType = (struct ValueType **)((char *)FuncValue->Val + sizeof(struct FuncDef));
    FuncValue->Val->FuncDef.ParamName = (char **)((char *)FuncValue->Val->FuncDef.ParamType + sizeof(struct ValueType *) * ParamCount);

    for (ParamCount = 0; ParamCount < FuncValue->Val->FuncDef.NumParams; ParamCount++)
    {

        if (ParamCount == FuncValue->Val->FuncDef.NumParams-1 && LexGetToken(&ParamParser, 0, 0) == TokenEllipsis)
        {

            FuncValue->Val->FuncDef.NumParams--;
            FuncValue->Val->FuncDef.VarArgs = 1;
            break;
        }
        else
        {

            TypeParse(&ParamParser, &ParamType, &ParamIdentifier, 0);
            if (ParamType->Base == TypeVoid)
            {

                ParamCount--;
                FuncValue->Val->FuncDef.NumParams--;
            }
            else
            {
                FuncValue->Val->FuncDef.ParamType[ParamCount] = ParamType;
                FuncValue->Val->FuncDef.ParamName[ParamCount] = ParamIdentifier;
            }
        }

        Token = LexGetToken(&ParamParser, 0, 1);
        if (Token != TokenComma && ParamCount < FuncValue->Val->FuncDef.NumParams-1)
            ProgramFail(&ParamParser, "comma expected");
    }

    if (FuncValue->Val->FuncDef.NumParams != 0 && Token != TokenCloseBracket && Token != TokenComma && Token != TokenEllipsis)
        ProgramFail(&ParamParser, "bad parameter");

    if (strcmp(Identifier, "main") == 0)
    {

        if ( FuncValue->Val->FuncDef.ReturnType != &pc->IntType &&
             FuncValue->Val->FuncDef.ReturnType != &pc->VoidType )
            ProgramFail(Parser, "main() should return an int or void");

        if (FuncValue->Val->FuncDef.NumParams != 0 &&
             (FuncValue->Val->FuncDef.NumParams != 2 || FuncValue->Val->FuncDef.ParamType[0] != &pc->IntType) )
            ProgramFail(Parser, "bad parameters to main()");
    }


    Token = LexGetToken(Parser, 0, 0);
    if (Token == TokenSemicolon)
        LexGetToken(Parser, 0, 1);
    else
    {

        if (Token != TokenLeftBrace)
            ProgramFail(Parser, "bad function definition");

        ParserCopy(&FuncBody, Parser);
        if (ParseStatementMaybeRun(Parser, 0, 1) != ParseResultOk)
            ProgramFail(Parser, "function definition expected");

        FuncValue->Val->FuncDef.Body = FuncBody;
        FuncValue->Val->FuncDef.Body.Pos = LexCopyTokens(&FuncBody, Parser);


        if (TableGet(&pc->GlobalTable, Identifier, &OldFuncValue, 0, 0, 0))
        {
            if (OldFuncValue->Val->FuncDef.Body.Pos == 0)
            {

                VariableFree(pc, TableDelete(pc, &pc->GlobalTable, Identifier));
            }
            else
                ProgramFail(Parser, "'%s' is already defined", Identifier);
        }
    }

    if (!TableSet(pc, &pc->GlobalTable, Identifier, FuncValue, (char *)Parser->FileName, Parser->Line, Parser->CharacterPos))
        ProgramFail(Parser, "'%s' is already defined", Identifier);

    return FuncValue;
}


int ParseArrayInitialiser(struct ParseState *Parser, struct Value *NewVariable, int DoAssignment)
{
    int ArrayIndex = 0;
    enum LexToken Token;
    struct Value *CValue;


    if (DoAssignment && Parser->Mode == RunModeRun)
    {
        struct ParseState CountParser;
        int NumElements;

        ParserCopy(&CountParser, Parser);
        NumElements = ParseArrayInitialiser(&CountParser, NewVariable, 0);

        if (NewVariable->Typ->Base != TypeArray)
            AssignFail(Parser, "%t from array initializer", NewVariable->Typ, 0, 0, 0, 0, 0);

        if (NewVariable->Typ->ArraySize == 0)
        {
            NewVariable->Typ = TypeGetMatching(Parser->pc, Parser, NewVariable->Typ->FromType, NewVariable->Typ->Base, NumElements, NewVariable->Typ->Identifier, 1);
            VariableRealloc(Parser, NewVariable, TypeSizeValue(NewVariable, 0));
        }




    }


    Token = LexGetToken(Parser, 0, 0);
    while (Token != TokenRightBrace)
    {
        if (LexGetToken(Parser, 0, 0) == TokenLeftBrace)
        {

            int SubArraySize = 0;
            struct Value *SubArray = NewVariable;
            if (Parser->Mode == RunModeRun && DoAssignment)
            {
                SubArraySize = TypeSize(NewVariable->Typ->FromType, NewVariable->Typ->FromType->ArraySize, 1);
                SubArray = VariableAllocValueFromExistingData(Parser, NewVariable->Typ->FromType, (union AnyValue *)(&NewVariable->Val->ArrayMem[0] + SubArraySize * ArrayIndex), 1, NewVariable);






                if (ArrayIndex >= NewVariable->Typ->ArraySize)
                    ProgramFail(Parser, "too many array elements");
            }
            LexGetToken(Parser, 0, 1);
            ParseArrayInitialiser(Parser, SubArray, DoAssignment);
        }
        else
        {
            struct Value *ArrayElement = 0;

            if (Parser->Mode == RunModeRun && DoAssignment)
            {
                struct ValueType * ElementType = NewVariable->Typ;
                int TotalSize = 1;
                int ElementSize = 0;


                while (ElementType->Base == TypeArray)
                {
                    TotalSize *= ElementType->ArraySize;
                    ElementType = ElementType->FromType;


                    if (LexGetToken(Parser, 0, 0) == TokenStringConstant && ElementType->FromType->Base == TypeChar)
                        break;
                }
                ElementSize = TypeSize(ElementType, ElementType->ArraySize, 1);




                if (ArrayIndex >= TotalSize)
                    ProgramFail(Parser, "too many array elements");
                ArrayElement = VariableAllocValueFromExistingData(Parser, ElementType, (union AnyValue *)(&NewVariable->Val->ArrayMem[0] + ElementSize * ArrayIndex), 1, NewVariable);
            }


            if (!ExpressionParse(Parser, &CValue))
                ProgramFail(Parser, "expression expected");

            if (Parser->Mode == RunModeRun && DoAssignment)
            {
                ExpressionAssign(Parser, ArrayElement, CValue, 0, 0, 0, 0);
                VariableStackPop(Parser, CValue);
                VariableStackPop(Parser, ArrayElement);
            }
        }

        ArrayIndex++;

        Token = LexGetToken(Parser, 0, 0);
        if (Token == TokenComma)
        {
            LexGetToken(Parser, 0, 1);
            Token = LexGetToken(Parser, 0, 0);
        }
        else if (Token != TokenRightBrace)
            ProgramFail(Parser, "comma expected");
    }

    if (Token == TokenRightBrace)
        LexGetToken(Parser, 0, 1);
    else
        ProgramFail(Parser, "'}' expected");

    return ArrayIndex;
}


void ParseDeclarationAssignment(struct ParseState *Parser, struct Value *NewVariable, int DoAssignment)
{
    struct Value *CValue;

    if (LexGetToken(Parser, 0, 0) == TokenLeftBrace)
    {

        LexGetToken(Parser, 0, 1);
        ParseArrayInitialiser(Parser, NewVariable, DoAssignment);
    }
    else
    {

        if (!ExpressionParse(Parser, &CValue))
            ProgramFail(Parser, "expression expected");

        if (Parser->Mode == RunModeRun && DoAssignment)
        {
            ExpressionAssign(Parser, NewVariable, CValue, 0, 0, 0, 0);
            VariableStackPop(Parser, CValue);
        }
    }
}


int ParseDeclaration(struct ParseState *Parser, enum LexToken Token)
{
    char *Identifier;
    struct ValueType *BasicType;
    struct ValueType *Typ;
    struct Value *NewVariable = 0;
    int IsStatic = 0;
    int FirstVisit = 0;
    Picoc *pc = Parser->pc;

    TypeParseFront(Parser, &BasicType, &IsStatic);
    do
    {
        TypeParseIdentPart(Parser, BasicType, &Typ, &Identifier);
        if ((Token != TokenVoidType && Token != TokenStructType && Token != TokenUnionType && Token != TokenEnumType) && Identifier == pc->StrEmpty)
            ProgramFail(Parser, "identifier expected");

        if (Identifier != pc->StrEmpty)
        {

            if (LexGetToken(Parser, 0, 0) == TokenOpenBracket)
            {
                ParseFunctionDefinition(Parser, Typ, Identifier);
                return 0;
            }
            else
            {
                if (Typ == &pc->VoidType && Identifier != pc->StrEmpty)
                    ProgramFail(Parser, "can't define a void variable");

                if (Parser->Mode == RunModeRun || Parser->Mode == RunModeGoto)
                    NewVariable = VariableDefineButIgnoreIdentical(Parser, Identifier, Typ, IsStatic, &FirstVisit);

                if (LexGetToken(Parser, 0, 0) == TokenAssign)
                {

                    LexGetToken(Parser, 0, 1);
                    ParseDeclarationAssignment(Parser, NewVariable, !IsStatic || FirstVisit);
                }
            }
        }

        Token = LexGetToken(Parser, 0, 0);
        if (Token == TokenComma)
            LexGetToken(Parser, 0, 1);

    } while (Token == TokenComma);

    return 1;
}


void ParseMacroDefinition(struct ParseState *Parser)
{
    struct Value *MacroName;
    char *MacroNameStr;
    struct Value *ParamName;
    struct Value *MacroValue;

    if (LexGetToken(Parser, &MacroName, 1) != TokenIdentifier)
        ProgramFail(Parser, "identifier expected");

    MacroNameStr = MacroName->Val->Identifier;

    if (LexRawPeekToken(Parser) == TokenOpenMacroBracket)
    {

        enum LexToken Token = LexGetToken(Parser, 0, 1);
        struct ParseState ParamParser;
        int NumParams;
        int ParamCount = 0;

        ParserCopy(&ParamParser, Parser);
        NumParams = ParseCountParams(&ParamParser);
        MacroValue = VariableAllocValueAndData(Parser->pc, Parser, sizeof(struct MacroDef) + sizeof(const char *) * NumParams, 0, 0, 1);
        MacroValue->Val->MacroDef.NumParams = NumParams;
        MacroValue->Val->MacroDef.ParamName = (char **)((char *)MacroValue->Val + sizeof(struct MacroDef));

        Token = LexGetToken(Parser, &ParamName, 1);

        while (Token == TokenIdentifier)
        {

            MacroValue->Val->MacroDef.ParamName[ParamCount++] = ParamName->Val->Identifier;


            Token = LexGetToken(Parser, 0, 1);
            if (Token == TokenComma)
                Token = LexGetToken(Parser, &ParamName, 1);

            else if (Token != TokenCloseBracket)
                ProgramFail(Parser, "comma expected");
        }

        if (Token != TokenCloseBracket)
            ProgramFail(Parser, "close bracket expected");
    }
    else
    {

        MacroValue = VariableAllocValueAndData(Parser->pc, Parser, sizeof(struct MacroDef), 0, 0, 1);
        MacroValue->Val->MacroDef.NumParams = 0;
    }


    ParserCopy(&MacroValue->Val->MacroDef.Body, Parser);
    MacroValue->Typ = &Parser->pc->MacroType;
    LexToEndOfLine(Parser);
    MacroValue->Val->MacroDef.Body.Pos = LexCopyTokens(&MacroValue->Val->MacroDef.Body, Parser);

    if (!TableSet(Parser->pc, &Parser->pc->GlobalTable, MacroNameStr, MacroValue, (char *)Parser->FileName, Parser->Line, Parser->CharacterPos))
        ProgramFail(Parser, "'%s' is already defined", MacroNameStr);
}


void ParserCopy(struct ParseState *To, struct ParseState *From)
{
    memcpy((void *)To, (void *)From, sizeof(*To));
}


void ParserCopyPos(struct ParseState *To, struct ParseState *From)
{
    To->Pos = From->Pos;
    To->Line = From->Line;
    To->HashIfLevel = From->HashIfLevel;
    To->HashIfEvaluateToLevel = From->HashIfEvaluateToLevel;
    To->CharacterPos = From->CharacterPos;
}


void ParseFor(struct ParseState *Parser)
{
    int Condition;
    struct ParseState PreConditional;
    struct ParseState PreIncrement;
    struct ParseState PreStatement;
    struct ParseState After;

    enum RunMode OldMode = Parser->Mode;

    int PrevScopeID = 0, ScopeID = VariableScopeBegin(Parser, &PrevScopeID);

    if (LexGetToken(Parser, 0, 1) != TokenOpenBracket)
        ProgramFail(Parser, "'(' expected");

    if (ParseStatement(Parser, 1) != ParseResultOk)
        ProgramFail(Parser, "statement expected");

    ParserCopyPos(&PreConditional, Parser);
    if (LexGetToken(Parser, 0, 0) == TokenSemicolon)
        Condition = 1;
    else
        Condition = ExpressionParseInt(Parser);

    if (LexGetToken(Parser, 0, 1) != TokenSemicolon)
        ProgramFail(Parser, "';' expected");

    ParserCopyPos(&PreIncrement, Parser);
    ParseStatementMaybeRun(Parser, 0, 0);

    if (LexGetToken(Parser, 0, 1) != TokenCloseBracket)
        ProgramFail(Parser, "')' expected");

    ParserCopyPos(&PreStatement, Parser);
    if (ParseStatementMaybeRun(Parser, Condition, 1) != ParseResultOk)
        ProgramFail(Parser, "statement expected");

    if (Parser->Mode == RunModeContinue && OldMode == RunModeRun)
        Parser->Mode = RunModeRun;

    ParserCopyPos(&After, Parser);

    while (Condition && Parser->Mode == RunModeRun)
    {
        ParserCopyPos(Parser, &PreIncrement);
        ParseStatement(Parser, 0);

        ParserCopyPos(Parser, &PreConditional);
        if (LexGetToken(Parser, 0, 0) == TokenSemicolon)
            Condition = 1;
        else
            Condition = ExpressionParseInt(Parser);

        if (Condition)
        {
            ParserCopyPos(Parser, &PreStatement);
            ParseStatement(Parser, 1);

            if (Parser->Mode == RunModeContinue)
                Parser->Mode = RunModeRun;
        }
    }

    if (Parser->Mode == RunModeBreak && OldMode == RunModeRun)
        Parser->Mode = RunModeRun;

    VariableScopeEnd(Parser, ScopeID, PrevScopeID);

    ParserCopyPos(Parser, &After);
}


enum RunMode ParseBlock(struct ParseState *Parser, int AbsorbOpenBrace, int Condition)
{
    int PrevScopeID = 0, ScopeID = VariableScopeBegin(Parser, &PrevScopeID);

    if (AbsorbOpenBrace && LexGetToken(Parser, 0, 1) != TokenLeftBrace)
        ProgramFail(Parser, "'{' expected");

    if (Parser->Mode == RunModeSkip || !Condition)
    {

        enum RunMode OldMode = Parser->Mode;
        Parser->Mode = RunModeSkip;
        while (ParseStatement(Parser, 1) == ParseResultOk)
        {}
        Parser->Mode = OldMode;
    }
    else
    {

        while (ParseStatement(Parser, 1) == ParseResultOk)
        {}
    }

    if (LexGetToken(Parser, 0, 1) != TokenRightBrace)
        ProgramFail(Parser, "'}' expected");

    VariableScopeEnd(Parser, ScopeID, PrevScopeID);

    return Parser->Mode;
}


void ParseTypedef(struct ParseState *Parser)
{
    struct ValueType *Typ;
    struct ValueType **TypPtr;
    char *TypeName;
    struct Value InitValue;

    TypeParse(Parser, &Typ, &TypeName, 0);

    if (Parser->Mode == RunModeRun)
    {
        TypPtr = &Typ;
        InitValue.Typ = &Parser->pc->TypeType;
        InitValue.Val = (union AnyValue *)TypPtr;
        VariableDefine(Parser->pc, Parser, TypeName, &InitValue, 0, 0);
    }
}


enum ParseResult ParseStatement(struct ParseState *Parser, int CheckTrailingSemicolon)
{
    struct Value *CValue;
    struct Value *LexerValue;
    struct Value *VarValue;
    int Condition;
    struct ParseState PreState;
    enum LexToken Token;


    if (Parser->DebugMode && Parser->Mode == RunModeRun)
        DebugCheckStatement(Parser);


    ParserCopy(&PreState, Parser);
    Token = LexGetToken(Parser, &LexerValue, 1);

    switch (Token)
    {
        case TokenEOF:
            return ParseResultEOF;

        case TokenIdentifier:

            if (VariableDefined(Parser->pc, LexerValue->Val->Identifier))
            {
                VariableGet(Parser->pc, Parser, LexerValue->Val->Identifier, &VarValue);
                if (VarValue->Typ->Base == Type_Type)
                {
                    *Parser = PreState;
                    ParseDeclaration(Parser, Token);
                    break;
                }
            }
            else
            {

                enum LexToken NextToken = LexGetToken(Parser, 0, 0);
                if (NextToken == TokenColon)
                {

                    LexGetToken(Parser, 0, 1);
                    if (Parser->Mode == RunModeGoto && LexerValue->Val->Identifier == Parser->SearchGotoLabel)
                        Parser->Mode = RunModeRun;

                    CheckTrailingSemicolon = 0;
                    break;
                }
//# 643 "c-demos/gitlab.com-zsaleeba-picoc/parse.c"
            }



        case TokenAsterisk:
        case TokenAmpersand:
        case TokenIncrement:
        case TokenDecrement:
        case TokenOpenBracket:
            *Parser = PreState;
            ExpressionParse(Parser, &CValue);
            if (Parser->Mode == RunModeRun)
                VariableStackPop(Parser, CValue);
            break;

        case TokenLeftBrace:
            ParseBlock(Parser, 0, 1);
            CheckTrailingSemicolon = 0;
            break;

        case TokenIf:
            if (LexGetToken(Parser, 0, 1) != TokenOpenBracket)
                ProgramFail(Parser, "'(' expected");

            Condition = ExpressionParseInt(Parser);

            if (LexGetToken(Parser, 0, 1) != TokenCloseBracket)
                ProgramFail(Parser, "')' expected");

            if (ParseStatementMaybeRun(Parser, Condition, 1) != ParseResultOk)
                ProgramFail(Parser, "statement expected");

            if (LexGetToken(Parser, 0, 0) == TokenElse)
            {
                LexGetToken(Parser, 0, 1);
                if (ParseStatementMaybeRun(Parser, !Condition, 1) != ParseResultOk)
                    ProgramFail(Parser, "statement expected");
            }
            CheckTrailingSemicolon = 0;
            break;

        case TokenWhile:
            {
                struct ParseState PreConditional;
                enum RunMode PreMode = Parser->Mode;

                if (LexGetToken(Parser, 0, 1) != TokenOpenBracket)
                    ProgramFail(Parser, "'(' expected");

                ParserCopyPos(&PreConditional, Parser);
                do
                {
                    ParserCopyPos(Parser, &PreConditional);
                    Condition = ExpressionParseInt(Parser);
                    if (LexGetToken(Parser, 0, 1) != TokenCloseBracket)
                        ProgramFail(Parser, "')' expected");

                    if (ParseStatementMaybeRun(Parser, Condition, 1) != ParseResultOk)
                        ProgramFail(Parser, "statement expected");

                    if (Parser->Mode == RunModeContinue)
                        Parser->Mode = PreMode;

                } while (Parser->Mode == RunModeRun && Condition);

                if (Parser->Mode == RunModeBreak)
                    Parser->Mode = PreMode;

                CheckTrailingSemicolon = 0;
            }
            break;

        case TokenDo:
            {
                struct ParseState PreStatement;
                enum RunMode PreMode = Parser->Mode;
                ParserCopyPos(&PreStatement, Parser);
                do
                {
                    ParserCopyPos(Parser, &PreStatement);
                    if (ParseStatement(Parser, 1) != ParseResultOk)
                        ProgramFail(Parser, "statement expected");

                    if (Parser->Mode == RunModeContinue)
                        Parser->Mode = PreMode;

                    if (LexGetToken(Parser, 0, 1) != TokenWhile)
                        ProgramFail(Parser, "'while' expected");

                    if (LexGetToken(Parser, 0, 1) != TokenOpenBracket)
                        ProgramFail(Parser, "'(' expected");

                    Condition = ExpressionParseInt(Parser);
                    if (LexGetToken(Parser, 0, 1) != TokenCloseBracket)
                        ProgramFail(Parser, "')' expected");

                } while (Condition && Parser->Mode == RunModeRun);

                if (Parser->Mode == RunModeBreak)
                    Parser->Mode = PreMode;
            }
            break;

        case TokenFor:
            ParseFor(Parser);
            CheckTrailingSemicolon = 0;
            break;

        case TokenSemicolon:
            CheckTrailingSemicolon = 0;
            break;

        case TokenIntType:
        case TokenShortType:
        case TokenCharType:
        case TokenLongType:
        case TokenFloatType:
        case TokenDoubleType:
        case TokenVoidType:
        case TokenStructType:
        case TokenUnionType:
        case TokenEnumType:
        case TokenSignedType:
        case TokenUnsignedType:
        case TokenStaticType:
        case TokenAutoType:
        case TokenRegisterType:
        case TokenExternType:
            *Parser = PreState;
            CheckTrailingSemicolon = ParseDeclaration(Parser, Token);
            break;

        case TokenHashDefine:
            ParseMacroDefinition(Parser);
            CheckTrailingSemicolon = 0;
            break;
//# 790 "c-demos/gitlab.com-zsaleeba-picoc/parse.c"
        case TokenSwitch:
            if (LexGetToken(Parser, 0, 1) != TokenOpenBracket)
                ProgramFail(Parser, "'(' expected");

            Condition = ExpressionParseInt(Parser);

            if (LexGetToken(Parser, 0, 1) != TokenCloseBracket)
                ProgramFail(Parser, "')' expected");

            if (LexGetToken(Parser, 0, 0) != TokenLeftBrace)
                ProgramFail(Parser, "'{' expected");

            {

                enum RunMode OldMode = Parser->Mode;
                int OldSearchLabel = Parser->SearchLabel;
                Parser->Mode = RunModeCaseSearch;
                Parser->SearchLabel = Condition;

                ParseBlock(Parser, 1, (OldMode != RunModeSkip) && (OldMode != RunModeReturn));

                if (Parser->Mode != RunModeReturn)
                    Parser->Mode = OldMode;

                Parser->SearchLabel = OldSearchLabel;
            }

            CheckTrailingSemicolon = 0;
            break;

        case TokenCase:
            if (Parser->Mode == RunModeCaseSearch)
            {
                Parser->Mode = RunModeRun;
                Condition = ExpressionParseInt(Parser);
                Parser->Mode = RunModeCaseSearch;
            }
            else
                Condition = ExpressionParseInt(Parser);

            if (LexGetToken(Parser, 0, 1) != TokenColon)
                ProgramFail(Parser, "':' expected");

            if (Parser->Mode == RunModeCaseSearch && Condition == Parser->SearchLabel)
                Parser->Mode = RunModeRun;

            CheckTrailingSemicolon = 0;
            break;

        case TokenDefault:
            if (LexGetToken(Parser, 0, 1) != TokenColon)
                ProgramFail(Parser, "':' expected");

            if (Parser->Mode == RunModeCaseSearch)
                Parser->Mode = RunModeRun;

            CheckTrailingSemicolon = 0;
            break;

        case TokenBreak:
            if (Parser->Mode == RunModeRun)
                Parser->Mode = RunModeBreak;
            break;

        case TokenContinue:
            if (Parser->Mode == RunModeRun)
                Parser->Mode = RunModeContinue;
            break;

        case TokenReturn:
            if (Parser->Mode == RunModeRun)
            {
                if (!Parser->pc->TopStackFrame || Parser->pc->TopStackFrame->ReturnValue->Typ->Base != TypeVoid)
                {
                    if (!ExpressionParse(Parser, &CValue))
                        ProgramFail(Parser, "value required in return");

                    if (!Parser->pc->TopStackFrame)
                        PlatformExit(Parser->pc, ExpressionCoerceInteger(CValue));
                    else
                        ExpressionAssign(Parser, Parser->pc->TopStackFrame->ReturnValue, CValue, 1, 0, 0, 0);

                    VariableStackPop(Parser, CValue);
                }
                else
                {
                    if (ExpressionParse(Parser, &CValue))
                        ProgramFail(Parser, "value in return from a void function");
                }

                Parser->Mode = RunModeReturn;
            }
            else
                ExpressionParse(Parser, &CValue);
            break;

        case TokenTypedef:
            ParseTypedef(Parser);
            break;

        case TokenGoto:
            if (LexGetToken(Parser, &LexerValue, 1) != TokenIdentifier)
                ProgramFail(Parser, "identifier expected");

            if (Parser->Mode == RunModeRun)
            {

                Parser->SearchGotoLabel = LexerValue->Val->Identifier;
                Parser->Mode = RunModeGoto;
            }
            break;

        case TokenDelete:
        {

            if (LexGetToken(Parser, &LexerValue, 1) != TokenIdentifier)
                ProgramFail(Parser, "identifier expected");

            if (Parser->Mode == RunModeRun)
            {

                CValue = TableDelete(Parser->pc, &Parser->pc->GlobalTable, LexerValue->Val->Identifier);

                if (CValue == 0)
                    ProgramFail(Parser, "'%s' is not defined", LexerValue->Val->Identifier);

                VariableFree(Parser->pc, CValue);
            }
            break;
        }

        default:
            *Parser = PreState;
            return ParseResultError;
    }

    if (CheckTrailingSemicolon)
    {
        if (LexGetToken(Parser, 0, 1) != TokenSemicolon)
            ProgramFail(Parser, "';' expected");
    }

    return ParseResultOk;
}


void PicocParse(Picoc *pc, const char *FileName, const char *Source, int SourceLen, int RunIt, int CleanupNow, int CleanupSource, int EnableDebugger)
{
    struct ParseState Parser;
    enum ParseResult Ok;
    struct CleanupTokenNode *NewCleanupNode;
    char *RegFileName = TableStrRegister(pc, FileName);

    void *Tokens = LexAnalyse(pc, RegFileName, Source, SourceLen, 0);


    if (!CleanupNow)
    {
        NewCleanupNode = HeapAllocMem(pc, sizeof(struct CleanupTokenNode));
        if (NewCleanupNode == 0)
            ProgramFailNoParser(pc, "out of memory");

        NewCleanupNode->Tokens = Tokens;
        if (CleanupSource)
            NewCleanupNode->SourceText = Source;
        else
            NewCleanupNode->SourceText = 0;

        NewCleanupNode->Next = pc->CleanupTokenList;
        pc->CleanupTokenList = NewCleanupNode;
    }


    LexInitParser(&Parser, pc, Source, Tokens, RegFileName, RunIt, EnableDebugger);

    do {
        Ok = ParseStatement(&Parser, 1);
    } while (Ok == ParseResultOk);

    if (Ok == ParseResultError)
        ProgramFail(&Parser, "parse error");


    if (CleanupNow)
        HeapFreeMem(pc, Tokens);
}


void PicocParseInteractiveNoStartPrompt(Picoc *pc, int EnableDebugger)
{
    struct ParseState Parser;
    enum ParseResult Ok;

    LexInitParser(&Parser, pc, 0, 0, pc->StrEmpty, 1, EnableDebugger);
    //PicocPlatformSetExitPoint(pc);
    LexInteractiveClear(pc, &Parser);

    do
    {
        LexInteractiveStatementPrompt(pc);
        Ok = ParseStatement(&Parser, 1);
        LexInteractiveCompleted(pc, &Parser);

    } while (Ok == ParseResultOk);

    if (Ok == ParseResultError)
        ProgramFail(&Parser, "parse error");

    PlatformPrintf(pc->CStdOut, "\n");
}


void PicocParseInteractive(Picoc *pc)
{
    PlatformPrintf(pc->CStdOut, "starting picoc " "v2.2" "\n");
    PicocParseInteractiveNoStartPrompt(pc, 1);
}
//# 8 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/picoc.c" 1

Picoc pc_object;

// int picoc(char *SourceStr)
int main()
{   
    char *pos;
    Picoc *pc = &pc_object;

    PicocInitialise(pc, HEAP_SIZE);

#if 0
    if (SourceStr)
    {
        for (pos = SourceStr; *pos != 0; pos++)
        {
            if (*pos == 0x1a)
            {
                *pos = 0x20;
            }
        }
    }

    PicocExitBuf[40] = 0;
    PicocPlatformSetExitPoint();
    if (PicocExitBuf[40]) {
        printf("Leaving PicoC\n\r");
        PicocCleanup();
        return PicocExitValue;
    }

    if (SourceStr)   
        PicocParse("nofile", SourceStr, strlen(SourceStr), TRUE, TRUE, FALSE);
#endif

    PicocParseInteractive(pc);
    PicocCleanup(pc);
    
    return 0;
}


//# 9 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/platform.c" 1
//# 9 "c-demos/gitlab.com-zsaleeba-picoc/platform.c"
void PicocInitialise(Picoc *pc, int StackSize)
{
    memset(pc, '\0', sizeof(*pc));
    //PlatformInit(pc);
    BasicIOInit(pc);
    HeapInit(pc, StackSize);
    TableInit(pc);
    VariableInit(pc);
    LexInit(pc);
    TypeInit(pc);



    LibraryInit(pc);

    LibraryAdd(pc, &pc->GlobalTable, "c library", &CLibrary[0]);
    CLibraryInit(pc);

    // PlatformLibraryInit(pc);
    DebugInit(pc);
}


void PicocCleanup(Picoc *pc)
{
    DebugCleanup(pc);



    ParseCleanup(pc);
    LexCleanup(pc);
    VariableCleanup(pc);
    TypeCleanup(pc);
    TableStrFree(pc);
    HeapCleanup(pc);
    // PlatformCleanup(pc);
}
//# 93 "c-demos/gitlab.com-zsaleeba-picoc/platform.c"
void PrintSourceTextErrorLine(IOFILE *Stream, const char *FileName, const char *SourceText, int Line, int CharacterPos)
{
    int LineCount;
    const char *LinePos;
    const char *CPos;
    int CCount;

    if (SourceText != 0)
    {

        for (LinePos = SourceText, LineCount = 1; *LinePos != '\0' && LineCount < Line; LinePos++)
        {
            if (*LinePos == '\n')
                LineCount++;
        }


        for (CPos = LinePos; *CPos != '\n' && *CPos != '\0'; CPos++)
            PrintCh(*CPos, Stream);
        PrintCh('\n', Stream);


        for (CPos = LinePos, CCount = 0; *CPos != '\n' && *CPos != '\0' && (CCount < CharacterPos || *CPos == ' '); CPos++, CCount++)
        {
            if (*CPos == '\t')
                PrintCh('\t', Stream);
            else
                PrintCh(' ', Stream);
        }
    }
    else
    {

        for (CCount = 0; CCount < CharacterPos + (int)strlen("picoc> "); CCount++)
            PrintCh(' ', Stream);
    }
    PlatformPrintf(Stream, "^\n%s:%d:%d ", FileName, Line, CharacterPos);

}


void ProgramFail(struct ParseState *Parser, const char *Message, ...)
{
    va_list Args;

    PrintSourceTextErrorLine(Parser->pc->CStdOut, Parser->FileName, Parser->SourceText, Parser->Line, Parser->CharacterPos);
    va_start(Args, Message);
    PlatformVPrintf(Parser->pc->CStdOut, Message, Args);
    va_end(Args);
    PlatformPrintf(Parser->pc->CStdOut, "\n");
    PlatformExit(Parser->pc, 1);
}


void ProgramFailNoParser(Picoc *pc, const char *Message, ...)
{
    va_list Args;

    va_start(Args, Message);
    PlatformVPrintf(pc->CStdOut, Message, Args);
    va_end(Args);
    PlatformPrintf(pc->CStdOut, "\n");
    PlatformExit(pc, 1);
}


void AssignFail(struct ParseState *Parser, const char *Format, struct ValueType *Type1, struct ValueType *Type2, int Num1, int Num2, const char *FuncName, int ParamNo)
{
    IOFILE *Stream = Parser->pc->CStdOut;

    PrintSourceTextErrorLine(Parser->pc->CStdOut, Parser->FileName, Parser->SourceText, Parser->Line, Parser->CharacterPos);
    PlatformPrintf(Stream, "can't %s ", (FuncName == 0) ? "assign" : "set");

    if (Type1 != 0)
        PlatformPrintf(Stream, Format, Type1, Type2);
    else
        PlatformPrintf(Stream, Format, Num1, Num2);

    if (FuncName != 0)
        PlatformPrintf(Stream, " in argument %d of call to %s()", ParamNo, FuncName);

    PlatformPrintf(Stream, "\n");
    PlatformExit(Parser->pc, 1);
}


void LexFail(Picoc *pc, struct LexState *Lexer, const char *Message, ...)
{
    va_list Args;

    PrintSourceTextErrorLine(pc->CStdOut, Lexer->FileName, Lexer->SourceText, Lexer->Line, Lexer->CharacterPos);
    va_start(Args, Message);
    PlatformVPrintf(pc->CStdOut, Message, Args);
    va_end(Args);
    PlatformPrintf(pc->CStdOut, "\n");
    PlatformExit(pc, 1);
}


void PlatformPrintf(IOFILE *Stream, const char *Format, ...)
{
    va_list Args;

    va_start(Args, Format);
    PlatformVPrintf(Stream, Format, Args);
    va_end(Args);
}

void PlatformVPrintf(IOFILE *Stream, const char *Format, va_list Args)
{
    const char *FPos;

    for (FPos = Format; *FPos != '\0'; FPos++)
    {
        if (*FPos == '%')
        {
            FPos++;
            switch (*FPos)
            {
            case 's': PrintStr(va_arg(Args, char *), Stream); break;
            case 'd': PrintSimpleInt(va_arg(Args, int), Stream); break;
            case 'c': PrintCh(va_arg(Args, int), Stream); break;
            case 't': PrintType(va_arg(Args, struct ValueType *), Stream); break;

            //case 'f': PrintFP(va_arg(Args, double), Stream); break;

            case '%': PrintCh('%', Stream); break;
            case '\0': FPos--; break;
            }
        }
        else
            PrintCh(*FPos, Stream);
    }
}



char *PlatformMakeTempName(Picoc *pc, char *TempNameBuffer)
{
    int CPos = 5;

    while (CPos > 1)
    {
        if (TempNameBuffer[CPos] < '9')
        {
            TempNameBuffer[CPos]++;
            return TableStrRegister(pc, TempNameBuffer);
        }
        else
        {
            TempNameBuffer[CPos] = '0';
            CPos--;
        }
    }

    return TableStrRegister(pc, TempNameBuffer);
}
//# 10 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/table.c" 1






void TableInit(Picoc *pc)
{
    TableInitTable(&pc->StringTable, &pc->StringHashTable[0], 97, 1);
    pc->StrEmpty = TableStrRegister(pc, "");
}


static unsigned int TableHash(const char *Key, int Len)
{
    unsigned int Hash = Len;
    int Offset;
    int Count;

    for (Count = 0, Offset = 8; Count < Len; Count++, Offset+=7)
    {
        if (Offset > sizeof(unsigned int) * 8 - 7)
            Offset -= sizeof(unsigned int) * 8 - 6;

        Hash ^= *Key++ << Offset;
    }

    return Hash;
}


void TableInitTable(struct Table *Tbl, struct TableEntry **HashTable, int Size, int OnHeap)
{
    Tbl->Size = Size;
    Tbl->OnHeap = OnHeap;
    Tbl->HashTable = HashTable;
    memset((void *)HashTable, '\0', sizeof(struct TableEntry *) * Size);
}


static struct TableEntry *TableSearch(struct Table *Tbl, const char *Key, int *AddAt)
{
    struct TableEntry *Entry;
    int HashValue = ((unsigned long)Key) % Tbl->Size;

    for (Entry = Tbl->HashTable[HashValue]; Entry != 0; Entry = Entry->Next)
    {
        if (Entry->p.v.Key == Key)
            return Entry;
    }

    *AddAt = HashValue;
    return 0;
}



int TableSet(Picoc *pc, struct Table *Tbl, char *Key, struct Value *Val, const char *DeclFileName, int DeclLine, int DeclColumn)
{
    int AddAt;
    struct TableEntry *FoundEntry = TableSearch(Tbl, Key, &AddAt);

    if (FoundEntry == 0)
    {
        struct TableEntry *NewEntry = VariableAlloc(pc, 0, sizeof(struct TableEntry), Tbl->OnHeap);
        NewEntry->DeclFileName = DeclFileName;
        NewEntry->DeclLine = DeclLine;
        NewEntry->DeclColumn = DeclColumn;
        NewEntry->p.v.Key = Key;
        NewEntry->p.v.Val = Val;
        NewEntry->Next = Tbl->HashTable[AddAt];
        Tbl->HashTable[AddAt] = NewEntry;
        return 1;
    }

    return 0;
}



int TableGet(struct Table *Tbl, const char *Key, struct Value **Val, const char **DeclFileName, int *DeclLine, int *DeclColumn)
{
    int AddAt;
    struct TableEntry *FoundEntry = TableSearch(Tbl, Key, &AddAt);
    if (FoundEntry == 0)
        return 0;

    *Val = FoundEntry->p.v.Val;

    if (DeclFileName != 0)
    {
        *DeclFileName = FoundEntry->DeclFileName;
        *DeclLine = FoundEntry->DeclLine;
        *DeclColumn = FoundEntry->DeclColumn;
    }

    return 1;
}


struct Value *TableDelete(Picoc *pc, struct Table *Tbl, const char *Key)
{
    struct TableEntry **EntryPtr;
    int HashValue = ((unsigned long)Key) % Tbl->Size;

    for (EntryPtr = &Tbl->HashTable[HashValue]; *EntryPtr != 0; EntryPtr = &(*EntryPtr)->Next)
    {
        if ((*EntryPtr)->p.v.Key == Key)
        {
            struct TableEntry *DeleteEntry = *EntryPtr;
            struct Value *Val = DeleteEntry->p.v.Val;
            *EntryPtr = DeleteEntry->Next;
            HeapFreeMem(pc, DeleteEntry);

            return Val;
        }
    }

    return 0;
}


static struct TableEntry *TableSearchIdentifier(struct Table *Tbl, const char *Key, int Len, int *AddAt)
{
    struct TableEntry *Entry;
    int HashValue = TableHash(Key, Len) % Tbl->Size;

    for (Entry = Tbl->HashTable[HashValue]; Entry != 0; Entry = Entry->Next)
    {
        if (strncmp(&Entry->p.Key[0], (char *)Key, Len) == 0 && Entry->p.Key[Len] == '\0')
            return Entry;
    }

    *AddAt = HashValue;
    return 0;
}


char *TableSetIdentifier(Picoc *pc, struct Table *Tbl, const char *Ident, int IdentLen)
{
    int AddAt;
    struct TableEntry *FoundEntry = TableSearchIdentifier(Tbl, Ident, IdentLen, &AddAt);

    if (FoundEntry != 0)
        return &FoundEntry->p.Key[0];
    else
    {
        struct TableEntry *NewEntry = HeapAllocMem(pc, sizeof(struct TableEntry) - sizeof(union TableEntryPayload) + IdentLen + 1);
        if (NewEntry == 0)
            ProgramFailNoParser(pc, "out of memory");

        strncpy((char *)&NewEntry->p.Key[0], (char *)Ident, IdentLen);
        NewEntry->p.Key[IdentLen] = '\0';
        NewEntry->Next = Tbl->HashTable[AddAt];
        Tbl->HashTable[AddAt] = NewEntry;
        return &NewEntry->p.Key[0];
    }
}


char *TableStrRegister2(Picoc *pc, const char *Str, int Len)
{
    return TableSetIdentifier(pc, &pc->StringTable, Str, Len);
}

char *TableStrRegister(Picoc *pc, const char *Str)
{
    return TableStrRegister2(pc, Str, strlen((char *)Str));
}


void TableStrFree(Picoc *pc)
{
    struct TableEntry *Entry;
    struct TableEntry *NextEntry;
    int Count;

    for (Count = 0; Count < pc->StringTable.Size; Count++)
    {
        for (Entry = pc->StringTable.HashTable[Count]; Entry != 0; Entry = NextEntry)
        {
            NextEntry = Entry->Next;
            HeapFreeMem(pc, Entry);
        }
    }
}
//# 11 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/type.c" 1






static int PointerAlignBytes;
static int IntAlignBytes;



struct ValueType *TypeAdd(Picoc *pc, struct ParseState *Parser, struct ValueType *ParentType, enum BaseType Base, int ArraySize, const char *Identifier, int Sizeof, int AlignBytes)
{
    struct ValueType *NewType = VariableAlloc(pc, Parser, sizeof(struct ValueType), 1);
    NewType->Base = Base;
    NewType->ArraySize = ArraySize;
    NewType->Sizeof = Sizeof;
    NewType->AlignBytes = AlignBytes;
    NewType->Identifier = Identifier;
    NewType->Members = 0;
    NewType->FromType = ParentType;
    NewType->DerivedTypeList = 0;
    NewType->OnHeap = 1;
    NewType->Next = ParentType->DerivedTypeList;
    ParentType->DerivedTypeList = NewType;

    return NewType;
}



struct ValueType *TypeGetMatching(Picoc *pc, struct ParseState *Parser, struct ValueType *ParentType, enum BaseType Base, int ArraySize, const char *Identifier, int AllowDuplicates)
{
    int Sizeof;
    int AlignBytes;
    struct ValueType *ThisType = ParentType->DerivedTypeList;
    while (ThisType != 0 && (ThisType->Base != Base || ThisType->ArraySize != ArraySize || ThisType->Identifier != Identifier))
        ThisType = ThisType->Next;

    if (ThisType != 0)
    {
        if (AllowDuplicates)
            return ThisType;
        else
            ProgramFail(Parser, "data type '%s' is already defined", Identifier);
    }

    switch (Base)
    {
        case TypePointer: Sizeof = sizeof(void *); AlignBytes = PointerAlignBytes; break;
        case TypeArray: Sizeof = ArraySize * ParentType->Sizeof; AlignBytes = ParentType->AlignBytes; break;
        case TypeEnum: Sizeof = sizeof(int); AlignBytes = IntAlignBytes; break;
        default: Sizeof = 0; AlignBytes = 0; break;
    }

    return TypeAdd(pc, Parser, ParentType, Base, ArraySize, Identifier, Sizeof, AlignBytes);
}


int TypeStackSizeValue(struct Value *Val)
{
    if (Val != 0 && Val->ValOnStack)
        return TypeSizeValue(Val, 0);
    else
        return 0;
}


int TypeSizeValue(struct Value *Val, int Compact)
{
    if ((((Val)->Typ)->Base >= TypeInt && ((Val)->Typ)->Base <= TypeUnsignedLong) && !Compact)
        return sizeof(char);
    else if (Val->Typ->Base != TypeArray)
        return Val->Typ->Sizeof;
    else
        return Val->Typ->FromType->Sizeof * Val->Typ->ArraySize;
}


int TypeSize(struct ValueType *Typ, int ArraySize, int Compact)
{
    if (((Typ)->Base >= TypeInt && (Typ)->Base <= TypeUnsignedLong) && !Compact)
        return sizeof(char);
    else if (Typ->Base != TypeArray)
        return Typ->Sizeof;
    else
        return Typ->FromType->Sizeof * ArraySize;
}


void TypeAddBaseType(Picoc *pc, struct ValueType *TypeNode, enum BaseType Base, int Sizeof, int AlignBytes)
{
    TypeNode->Base = Base;
    TypeNode->ArraySize = 0;
    TypeNode->Sizeof = Sizeof;
    TypeNode->AlignBytes = AlignBytes;
    TypeNode->Identifier = pc->StrEmpty;
    TypeNode->Members = 0;
    TypeNode->FromType = 0;
    TypeNode->DerivedTypeList = 0;
    TypeNode->OnHeap = 0;
    TypeNode->Next = pc->UberType.DerivedTypeList;
    pc->UberType.DerivedTypeList = TypeNode;
}

struct IntAlign { char x; int y; };
struct ShortAlign { char x; short y; };
struct CharAlign { char x; char y; };
struct LongAlign { char x; long y; };
struct PointerAlign { char x; void *y; };


void TypeInit(Picoc *pc)
{
    struct IntAlign  ia;
    struct ShortAlign  sa;
    struct CharAlign  ca;
    struct LongAlign  la;

    struct PointerAlign  pa;

    IntAlignBytes = (char *)&ia.y - &ia.x;
    PointerAlignBytes = (char *)&pa.y - &pa.x;

    pc->UberType.DerivedTypeList = 0;
    TypeAddBaseType(pc, &pc->IntType, TypeInt, sizeof(int), IntAlignBytes);
    TypeAddBaseType(pc, &pc->ShortType, TypeShort, sizeof(short), (char *)&sa.y - &sa.x);
    TypeAddBaseType(pc, &pc->CharType, TypeChar, sizeof(char), (char *)&ca.y - &ca.x);
    TypeAddBaseType(pc, &pc->LongType, TypeLong, sizeof(long), (char *)&la.y - &la.x);
    TypeAddBaseType(pc, &pc->UnsignedIntType, TypeUnsignedInt, sizeof(unsigned int), IntAlignBytes);
    TypeAddBaseType(pc, &pc->UnsignedShortType, TypeUnsignedShort, sizeof(unsigned short), (char *)&sa.y - &sa.x);
    TypeAddBaseType(pc, &pc->UnsignedLongType, TypeUnsignedLong, sizeof(unsigned long), (char *)&la.y - &la.x);
    TypeAddBaseType(pc, &pc->UnsignedCharType, TypeUnsignedChar, sizeof(unsigned char), (char *)&ca.y - &ca.x);
    TypeAddBaseType(pc, &pc->VoidType, TypeVoid, 0, 1);
    TypeAddBaseType(pc, &pc->FunctionType, TypeFunction, sizeof(int), IntAlignBytes);
    TypeAddBaseType(pc, &pc->MacroType, TypeMacro, sizeof(int), IntAlignBytes);
    TypeAddBaseType(pc, &pc->GotoLabelType, TypeGotoLabel, 0, 1);

    //TypeAddBaseType(pc, &pc->FPType, TypeFP, sizeof(double), (char *)&da.y - &da.x);
    // TypeAddBaseType(pc, &pc->TypeType, Type_Type, sizeof(double), (char *)&da.y - &da.x);



    pc->CharArrayType = TypeAdd(pc, 0, &pc->CharType, TypeArray, 0, pc->StrEmpty, sizeof(char), (char *)&ca.y - &ca.x);
    pc->CharPtrType = TypeAdd(pc, 0, &pc->CharType, TypePointer, 0, pc->StrEmpty, sizeof(void *), PointerAlignBytes);
    pc->CharPtrPtrType = TypeAdd(pc, 0, pc->CharPtrType, TypePointer, 0, pc->StrEmpty, sizeof(void *), PointerAlignBytes);
    pc->VoidPtrType = TypeAdd(pc, 0, &pc->VoidType, TypePointer, 0, pc->StrEmpty, sizeof(void *), PointerAlignBytes);
}


void TypeCleanupNode(Picoc *pc, struct ValueType *Typ)
{
    struct ValueType *SubType;
    struct ValueType *NextSubType;


    for (SubType = Typ->DerivedTypeList; SubType != 0; SubType = NextSubType)
    {
        NextSubType = SubType->Next;
        TypeCleanupNode(pc, SubType);
        if (SubType->OnHeap)
        {

            if (SubType->Members != 0)
            {
                VariableTableCleanup(pc, SubType->Members);
                HeapFreeMem(pc, SubType->Members);
            }


            HeapFreeMem(pc, SubType);
        }
    }
}

void TypeCleanup(Picoc *pc)
{
    TypeCleanupNode(pc, &pc->UberType);
}

static char TempNameBuf[7] = "^s0000";

void TypeParseStruct(struct ParseState *Parser, struct ValueType **Typ, int IsStruct)
{
    struct Value *LexValue;
    struct ValueType *MemberType;
    char *MemberIdentifier;
    char *StructIdentifier;
    struct Value *MemberValue;
    enum LexToken Token;
    int AlignBoundary;
    Picoc *pc = Parser->pc;

    Token = LexGetToken(Parser, &LexValue, 0);
    if (Token == TokenIdentifier)
    {
        LexGetToken(Parser, &LexValue, 1);
        StructIdentifier = LexValue->Val->Identifier;
        Token = LexGetToken(Parser, 0, 0);
    }
    else
    {
        StructIdentifier = PlatformMakeTempName(pc, TempNameBuf);
    }

    *Typ = TypeGetMatching(pc, Parser, &Parser->pc->UberType, IsStruct ? TypeStruct : TypeUnion, 0, StructIdentifier, 1);
    if (Token == TokenLeftBrace && (*Typ)->Members != 0)
        ProgramFail(Parser, "data type '%t' is already defined", *Typ);

    Token = LexGetToken(Parser, 0, 0);
    if (Token != TokenLeftBrace)
    {





        return;
    }

    if (pc->TopStackFrame != 0)
        ProgramFail(Parser, "struct/union definitions can only be globals");

    LexGetToken(Parser, 0, 1);
    (*Typ)->Members = VariableAlloc(pc, Parser, sizeof(struct Table) + 11 * sizeof(struct TableEntry), 1);
    (*Typ)->Members->HashTable = (struct TableEntry **)((char *)(*Typ)->Members + sizeof(struct Table));
    TableInitTable((*Typ)->Members, (struct TableEntry **)((char *)(*Typ)->Members + sizeof(struct Table)), 11, 1);

    do {
        TypeParse(Parser, &MemberType, &MemberIdentifier, 0);
        if (MemberType == 0 || MemberIdentifier == 0)
            ProgramFail(Parser, "invalid type in struct");

        MemberValue = VariableAllocValueAndData(pc, Parser, sizeof(int), 0, 0, 1);
        MemberValue->Typ = MemberType;
        if (IsStruct)
        {

            AlignBoundary = MemberValue->Typ->AlignBytes;
            if (((*Typ)->Sizeof & (AlignBoundary-1)) != 0)
                (*Typ)->Sizeof += AlignBoundary - ((*Typ)->Sizeof & (AlignBoundary-1));

            MemberValue->Val->Integer = (*Typ)->Sizeof;
            (*Typ)->Sizeof += TypeSizeValue(MemberValue, 1);
        }
        else
        {

            MemberValue->Val->Integer = 0;
            if (MemberValue->Typ->Sizeof > (*Typ)->Sizeof)
                (*Typ)->Sizeof = TypeSizeValue(MemberValue, 1);
        }


        if ((*Typ)->AlignBytes < MemberValue->Typ->AlignBytes)
            (*Typ)->AlignBytes = MemberValue->Typ->AlignBytes;


        if (!TableSet(pc, (*Typ)->Members, MemberIdentifier, MemberValue, Parser->FileName, Parser->Line, Parser->CharacterPos))
            ProgramFail(Parser, "member '%s' already defined", &MemberIdentifier);

        if (LexGetToken(Parser, 0, 1) != TokenSemicolon)
            ProgramFail(Parser, "semicolon expected");

    } while (LexGetToken(Parser, 0, 0) != TokenRightBrace);


    AlignBoundary = (*Typ)->AlignBytes;
    if (((*Typ)->Sizeof & (AlignBoundary-1)) != 0)
        (*Typ)->Sizeof += AlignBoundary - ((*Typ)->Sizeof & (AlignBoundary-1));

    LexGetToken(Parser, 0, 1);
}


struct ValueType *TypeCreateOpaqueStruct(Picoc *pc, struct ParseState *Parser, const char *StructName, int Size)
{
    struct ValueType *Typ = TypeGetMatching(pc, Parser, &pc->UberType, TypeStruct, 0, StructName, 0);


    Typ->Members = VariableAlloc(pc, Parser, sizeof(struct Table) + 11 * sizeof(struct TableEntry), 1);
    Typ->Members->HashTable = (struct TableEntry **)((char *)Typ->Members + sizeof(struct Table));
    TableInitTable(Typ->Members, (struct TableEntry **)((char *)Typ->Members + sizeof(struct Table)), 11, 1);
    Typ->Sizeof = Size;

    return Typ;
}

static char TempNameBuf2[7] = "^e0000";

void TypeParseEnum(struct ParseState *Parser, struct ValueType **Typ)
{
    struct Value *LexValue;
    struct Value InitValue;
    enum LexToken Token;
    int EnumValue = 0;
    char *EnumIdentifier;
    Picoc *pc = Parser->pc;

    Token = LexGetToken(Parser, &LexValue, 0);
    if (Token == TokenIdentifier)
    {
        LexGetToken(Parser, &LexValue, 1);
        EnumIdentifier = LexValue->Val->Identifier;
        Token = LexGetToken(Parser, 0, 0);
    }
    else
    {
        EnumIdentifier = PlatformMakeTempName(pc, TempNameBuf2);
    }

    TypeGetMatching(pc, Parser, &pc->UberType, TypeEnum, 0, EnumIdentifier, Token != TokenLeftBrace);
    *Typ = &pc->IntType;
    if (Token != TokenLeftBrace)
    {

        if ((*Typ)->Members == 0)
            ProgramFail(Parser, "enum '%s' isn't defined", EnumIdentifier);

        return;
    }

    if (pc->TopStackFrame != 0)
        ProgramFail(Parser, "enum definitions can only be globals");

    LexGetToken(Parser, 0, 1);
    (*Typ)->Members = &pc->GlobalTable;
    memset((void *)&InitValue, '\0', sizeof(struct Value));
    InitValue.Typ = &pc->IntType;
    InitValue.Val = (union AnyValue *)&EnumValue;
    do {
        if (LexGetToken(Parser, &LexValue, 1) != TokenIdentifier)
            ProgramFail(Parser, "identifier expected");

        EnumIdentifier = LexValue->Val->Identifier;
        if (LexGetToken(Parser, 0, 0) == TokenAssign)
        {
            LexGetToken(Parser, 0, 1);
            EnumValue = ExpressionParseInt(Parser);
        }

        VariableDefine(pc, Parser, EnumIdentifier, &InitValue, 0, 0);

        Token = LexGetToken(Parser, 0, 1);
        if (Token != TokenComma && Token != TokenRightBrace)
            ProgramFail(Parser, "comma expected");

        EnumValue++;

    } while (Token == TokenComma);
}


int TypeParseFront(struct ParseState *Parser, struct ValueType **Typ, int *IsStatic)
{
    struct ParseState Before;
    struct Value *LexerValue;
    enum LexToken Token;
    int Unsigned = 0;
    struct Value *VarValue;
    int StaticQualifier = 0;
    Picoc *pc = Parser->pc;
    *Typ = 0;


    ParserCopy(&Before, Parser);
    Token = LexGetToken(Parser, &LexerValue, 1);
    while (Token == TokenStaticType || Token == TokenAutoType || Token == TokenRegisterType || Token == TokenExternType)
    {
        if (Token == TokenStaticType)
            StaticQualifier = 1;

        Token = LexGetToken(Parser, &LexerValue, 1);
    }

    if (IsStatic != 0)
        *IsStatic = StaticQualifier;


    if (Token == TokenSignedType || Token == TokenUnsignedType)
    {
        enum LexToken FollowToken = LexGetToken(Parser, &LexerValue, 0);
        Unsigned = (Token == TokenUnsignedType);

        if (FollowToken != TokenIntType && FollowToken != TokenLongType && FollowToken != TokenShortType && FollowToken != TokenCharType)
        {
            if (Token == TokenUnsignedType)
                *Typ = &pc->UnsignedIntType;
            else
                *Typ = &pc->IntType;

            return 1;
        }

        Token = LexGetToken(Parser, &LexerValue, 1);
    }

    switch (Token)
    {
        case TokenIntType: *Typ = Unsigned ? &pc->UnsignedIntType : &pc->IntType; break;
        case TokenShortType: *Typ = Unsigned ? &pc->UnsignedShortType : &pc->ShortType; break;
        case TokenCharType: *Typ = Unsigned ? &pc->UnsignedCharType : &pc->CharType; break;
        case TokenLongType: *Typ = Unsigned ? &pc->UnsignedLongType : &pc->LongType; break;

        //case TokenFloatType: case TokenDoubleType: *Typ = &pc->FPType; break;

        case TokenVoidType: *Typ = &pc->VoidType; break;

        case TokenStructType: case TokenUnionType:
            if (*Typ != 0)
                ProgramFail(Parser, "bad type declaration");

            TypeParseStruct(Parser, Typ, Token == TokenStructType);
            break;

        case TokenEnumType:
            if (*Typ != 0)
                ProgramFail(Parser, "bad type declaration");

            TypeParseEnum(Parser, Typ);
            break;

        case TokenIdentifier:

            VariableGet(pc, Parser, LexerValue->Val->Identifier, &VarValue);
            *Typ = VarValue->Val->Typ;
            break;

        default: ParserCopy(Parser, &Before); return 0;
    }

    return 1;
}


struct ValueType *TypeParseBack(struct ParseState *Parser, struct ValueType *FromType)
{
    enum LexToken Token;
    struct ParseState Before;

    ParserCopy(&Before, Parser);
    Token = LexGetToken(Parser, 0, 1);
    if (Token == TokenLeftSquareBracket)
    {

        if (LexGetToken(Parser, 0, 0) == TokenRightSquareBracket)
        {

            LexGetToken(Parser, 0, 1);
            return TypeGetMatching(Parser->pc, Parser, TypeParseBack(Parser, FromType), TypeArray, 0, Parser->pc->StrEmpty, 1);
        }
        else
        {

            enum RunMode OldMode = Parser->Mode;
            int ArraySize;
            Parser->Mode = RunModeRun;
            ArraySize = ExpressionParseInt(Parser);
            Parser->Mode = OldMode;

            if (LexGetToken(Parser, 0, 1) != TokenRightSquareBracket)
                ProgramFail(Parser, "']' expected");

            return TypeGetMatching(Parser->pc, Parser, TypeParseBack(Parser, FromType), TypeArray, ArraySize, Parser->pc->StrEmpty, 1);
        }
    }
    else
    {

        ParserCopy(Parser, &Before);
        return FromType;
    }
}


void TypeParseIdentPart(struct ParseState *Parser, struct ValueType *BasicTyp, struct ValueType **Typ, char **Identifier)
{
    struct ParseState Before;
    enum LexToken Token;
    struct Value *LexValue;
    int Done = 0;
    *Typ = BasicTyp;
    *Identifier = Parser->pc->StrEmpty;

    while (!Done)
    {
        ParserCopy(&Before, Parser);
        Token = LexGetToken(Parser, &LexValue, 1);
        switch (Token)
        {
            case TokenOpenBracket:
                if (*Typ != 0)
                    ProgramFail(Parser, "bad type declaration");

                TypeParse(Parser, Typ, Identifier, 0);
                if (LexGetToken(Parser, 0, 1) != TokenCloseBracket)
                    ProgramFail(Parser, "')' expected");
                break;

            case TokenAsterisk:
                if (*Typ == 0)
                    ProgramFail(Parser, "bad type declaration");

                *Typ = TypeGetMatching(Parser->pc, Parser, *Typ, TypePointer, 0, Parser->pc->StrEmpty, 1);
                break;

            case TokenIdentifier:
                if (*Typ == 0 || *Identifier != Parser->pc->StrEmpty)
                    ProgramFail(Parser, "bad type declaration");

                *Identifier = LexValue->Val->Identifier;
                Done = 1;
                break;

            default: ParserCopy(Parser, &Before); Done = 1; break;
        }
    }

    if (*Typ == 0)
        ProgramFail(Parser, "bad type declaration");

    if (*Identifier != Parser->pc->StrEmpty)
    {

        *Typ = TypeParseBack(Parser, *Typ);
    }
}


void TypeParse(struct ParseState *Parser, struct ValueType **Typ, char **Identifier, int *IsStatic)
{
    struct ValueType *BasicType;

    TypeParseFront(Parser, &BasicType, IsStatic);
    TypeParseIdentPart(Parser, BasicType, Typ, Identifier);
}


int TypeIsForwardDeclared(struct ParseState *Parser, struct ValueType *Typ)
{
    if (Typ->Base == TypeArray)
        return TypeIsForwardDeclared(Parser, Typ->FromType);

    if ( (Typ->Base == TypeStruct || Typ->Base == TypeUnion) && Typ->Members == 0)
        return 1;

    return 0;
}
//# 12 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
//# 1 "c-demos/gitlab.com-zsaleeba-picoc/variable.c" 1
//# 11 "c-demos/gitlab.com-zsaleeba-picoc/variable.c"
void VariableInit(Picoc *pc)
{
    TableInitTable(&(pc->GlobalTable), &(pc->GlobalHashTable)[0], 97, 1);
    TableInitTable(&pc->StringLiteralTable, &pc->StringLiteralHashTable[0], 97, 1);
    pc->TopStackFrame = 0;
}


void VariableFree(Picoc *pc, struct Value *Val)
{
    if (Val->ValOnHeap || Val->AnyValOnHeap)
    {

        if (Val->Typ == &pc->FunctionType && Val->Val->FuncDef.Intrinsic == 0 && Val->Val->FuncDef.Body.Pos != 0)
            HeapFreeMem(pc, (void *)Val->Val->FuncDef.Body.Pos);


        if (Val->Typ == &pc->MacroType)
            HeapFreeMem(pc, (void *)Val->Val->MacroDef.Body.Pos);


        if (Val->AnyValOnHeap)
            HeapFreeMem(pc, Val->Val);
    }


    if (Val->ValOnHeap)
        HeapFreeMem(pc, Val);
}


void VariableTableCleanup(Picoc *pc, struct Table *HashTable)
{
    struct TableEntry *Entry;
    struct TableEntry *NextEntry;
    int Count;

    for (Count = 0; Count < HashTable->Size; Count++)
    {
        for (Entry = HashTable->HashTable[Count]; Entry != 0; Entry = NextEntry)
        {
            NextEntry = Entry->Next;
            VariableFree(pc, Entry->p.v.Val);


            HeapFreeMem(pc, Entry);
        }
    }
}

void VariableCleanup(Picoc *pc)
{
    VariableTableCleanup(pc, &pc->GlobalTable);
    VariableTableCleanup(pc, &pc->StringLiteralTable);
}


void *VariableAlloc(Picoc *pc, struct ParseState *Parser, int Size, int OnHeap)
{
    void *NewValue;

    if (OnHeap)
        NewValue = HeapAllocMem(pc, Size);
    else
        NewValue = HeapAllocStack(pc, Size);

    if (NewValue == 0)
        ProgramFail(Parser, "out of memory");






    return NewValue;
}


struct Value *VariableAllocValueAndData(Picoc *pc, struct ParseState *Parser, int DataSize, int IsLValue, struct Value *LValueFrom, int OnHeap)
{
    struct Value *NewValue = VariableAlloc(pc, Parser, (((sizeof(struct Value)) + sizeof(char) - 1) & ~(sizeof(char)-1)) + DataSize, OnHeap);
    NewValue->Val = (union AnyValue *)((char *)NewValue + (((sizeof(struct Value)) + sizeof(char) - 1) & ~(sizeof(char)-1)));
    NewValue->ValOnHeap = OnHeap;
    NewValue->AnyValOnHeap = 0;
    NewValue->ValOnStack = !OnHeap;
    NewValue->IsLValue = IsLValue;
    NewValue->LValueFrom = LValueFrom;
    if (Parser)
        NewValue->ScopeID = Parser->ScopeID;

    NewValue->OutOfScope = 0;

    return NewValue;
}


struct Value *VariableAllocValueFromType(Picoc *pc, struct ParseState *Parser, struct ValueType *Typ, int IsLValue, struct Value *LValueFrom, int OnHeap)
{
    int Size = TypeSize(Typ, Typ->ArraySize, 0);
    struct Value *NewValue = VariableAllocValueAndData(pc, Parser, Size, IsLValue, LValueFrom, OnHeap);
    assert(Size >= 0 || Typ == &pc->VoidType);
    NewValue->Typ = Typ;

    return NewValue;
}


struct Value *VariableAllocValueAndCopy(Picoc *pc, struct ParseState *Parser, struct Value *FromValue, int OnHeap)
{
    struct ValueType *DType = FromValue->Typ;
    struct Value *NewValue;
    char TmpBuf[256];
    int CopySize = TypeSizeValue(FromValue, 1);

    assert(CopySize <= 256);
    memcpy((void *)&TmpBuf[0], (void *)FromValue->Val, CopySize);
    NewValue = VariableAllocValueAndData(pc, Parser, CopySize, FromValue->IsLValue, FromValue->LValueFrom, OnHeap);
    NewValue->Typ = DType;
    memcpy((void *)NewValue->Val, (void *)&TmpBuf[0], CopySize);

    return NewValue;
}


struct Value *VariableAllocValueFromExistingData(struct ParseState *Parser, struct ValueType *Typ, union AnyValue *FromValue, int IsLValue, struct Value *LValueFrom)
{
    struct Value *NewValue = VariableAlloc(Parser->pc, Parser, sizeof(struct Value), 0);
    NewValue->Typ = Typ;
    NewValue->Val = FromValue;
    NewValue->ValOnHeap = 0;
    NewValue->AnyValOnHeap = 0;
    NewValue->ValOnStack = 0;
    NewValue->IsLValue = IsLValue;
    NewValue->LValueFrom = LValueFrom;

    return NewValue;
}


struct Value *VariableAllocValueShared(struct ParseState *Parser, struct Value *FromValue)
{
    return VariableAllocValueFromExistingData(Parser, FromValue->Typ, FromValue->Val, FromValue->IsLValue, FromValue->IsLValue ? FromValue : 0);
}


void VariableRealloc(struct ParseState *Parser, struct Value *FromValue, int NewSize)
{
    if (FromValue->AnyValOnHeap)
        HeapFreeMem(Parser->pc, FromValue->Val);

    FromValue->Val = VariableAlloc(Parser->pc, Parser, NewSize, 1);
    FromValue->AnyValOnHeap = 1;
}

int VariableScopeBegin(struct ParseState * Parser, int* OldScopeID)
{
    struct TableEntry *Entry;
    struct TableEntry *NextEntry;
    Picoc * pc = Parser->pc;
    int Count;




    struct Table * HashTable = (pc->TopStackFrame == 0) ? &(pc->GlobalTable) : &(pc->TopStackFrame)->LocalTable;

    if (Parser->ScopeID == -1) return -1;


    *OldScopeID = Parser->ScopeID;
    Parser->ScopeID = (int)(intptr_t)(Parser->SourceText) * ((int)(intptr_t)(Parser->Pos) / sizeof(char*));



    for (Count = 0; Count < HashTable->Size; Count++)
    {
        for (Entry = HashTable->HashTable[Count]; Entry != 0; Entry = NextEntry)
        {
            NextEntry = Entry->Next;
            if (Entry->p.v.Val->ScopeID == Parser->ScopeID && Entry->p.v.Val->OutOfScope)
            {
                Entry->p.v.Val->OutOfScope = 0;
                Entry->p.v.Key = (char*)((intptr_t)Entry->p.v.Key & ~1);





            }
        }
    }

    return Parser->ScopeID;
}

void VariableScopeEnd(struct ParseState * Parser, int ScopeID, int PrevScopeID)
{
    struct TableEntry *Entry;
    struct TableEntry *NextEntry;
    Picoc * pc = Parser->pc;
    int Count;




    struct Table * HashTable = (pc->TopStackFrame == 0) ? &(pc->GlobalTable) : &(pc->TopStackFrame)->LocalTable;

    if (ScopeID == -1) return;

    for (Count = 0; Count < HashTable->Size; Count++)
    {
        for (Entry = HashTable->HashTable[Count]; Entry != 0; Entry = NextEntry)
        {
            NextEntry = Entry->Next;
            if (Entry->p.v.Val->ScopeID == ScopeID && !Entry->p.v.Val->OutOfScope)
            {





                Entry->p.v.Val->OutOfScope = 1;
                Entry->p.v.Key = (char*)((intptr_t)Entry->p.v.Key | 1);
            }
        }
    }

    Parser->ScopeID = PrevScopeID;
}

int VariableDefinedAndOutOfScope(Picoc * pc, const char* Ident)
{
    struct TableEntry *Entry;
    int Count;

    struct Table * HashTable = (pc->TopStackFrame == 0) ? &(pc->GlobalTable) : &(pc->TopStackFrame)->LocalTable;
    for (Count = 0; Count < HashTable->Size; Count++)
    {
        for (Entry = HashTable->HashTable[Count]; Entry != 0; Entry = Entry->Next)
        {
            if (Entry->p.v.Val->OutOfScope && (char*)((intptr_t)Entry->p.v.Key & ~1) == Ident)
                return 1;
        }
    }
    return 0;
}


struct Value *VariableDefine(Picoc *pc, struct ParseState *Parser, char *Ident, struct Value *InitValue, struct ValueType *Typ, int MakeWritable)
{
    struct Value * AssignValue;
    struct Table * currentTable = (pc->TopStackFrame == 0) ? &(pc->GlobalTable) : &(pc->TopStackFrame)->LocalTable;

    int ScopeID = Parser ? Parser->ScopeID : -1;




    if (InitValue != 0)
        AssignValue = VariableAllocValueAndCopy(pc, Parser, InitValue, pc->TopStackFrame == 0);
    else
        AssignValue = VariableAllocValueFromType(pc, Parser, Typ, MakeWritable, 0, pc->TopStackFrame == 0);

    AssignValue->IsLValue = MakeWritable;
    AssignValue->ScopeID = ScopeID;
    AssignValue->OutOfScope = 0;

    if (!TableSet(pc, currentTable, Ident, AssignValue, Parser ? ((char *)Parser->FileName) : 0, Parser ? Parser->Line : 0, Parser ? Parser->CharacterPos : 0))
        ProgramFail(Parser, "'%s' is already defined", Ident);

    return AssignValue;
}


struct Value *VariableDefineButIgnoreIdentical(struct ParseState *Parser, char *Ident, struct ValueType *Typ, int IsStatic, int *FirstVisit)
{
    Picoc *pc = Parser->pc;
    struct Value *ExistingValue;
    const char *DeclFileName;
    int DeclLine;
    int DeclColumn;


    if (TypeIsForwardDeclared(Parser, Typ))
        ProgramFail(Parser, "type '%t' isn't defined", Typ);

    if (IsStatic)
    {
        char MangledName[256];
        char *MNPos = &MangledName[0];
        char *MNEnd = &MangledName[256 -1];
        const char *RegisteredMangledName;


        memset((void *)&MangledName, '\0', sizeof(MangledName));
        *MNPos++ = '/';
        strncpy(MNPos, (char *)Parser->FileName, MNEnd - MNPos);
        MNPos += strlen(MNPos);

        if (pc->TopStackFrame != 0)
        {

            if (MNEnd - MNPos > 0) *MNPos++ = '/';
            strncpy(MNPos, (char *)pc->TopStackFrame->FuncName, MNEnd - MNPos);
            MNPos += strlen(MNPos);
        }

        if (MNEnd - MNPos > 0) *MNPos++ = '/';
        strncpy(MNPos, Ident, MNEnd - MNPos);
        RegisteredMangledName = TableStrRegister(pc, MangledName);


        if (!TableGet(&pc->GlobalTable, RegisteredMangledName, &ExistingValue, &DeclFileName, &DeclLine, &DeclColumn))
        {

            ExistingValue = VariableAllocValueFromType(Parser->pc, Parser, Typ, 1, 0, 1);
            TableSet(pc, &pc->GlobalTable, (char *)RegisteredMangledName, ExistingValue, (char *)Parser->FileName, Parser->Line, Parser->CharacterPos);
            *FirstVisit = 1;
        }


        VariableDefinePlatformVar(Parser->pc, Parser, Ident, ExistingValue->Typ, ExistingValue->Val, 1);
        return ExistingValue;
    }
    else
    {
        if (Parser->Line != 0 && TableGet((pc->TopStackFrame == 0) ? &pc->GlobalTable : &pc->TopStackFrame->LocalTable, Ident, &ExistingValue, &DeclFileName, &DeclLine, &DeclColumn)
                && DeclFileName == Parser->FileName && DeclLine == Parser->Line && DeclColumn == Parser->CharacterPos)
            return ExistingValue;
        else
            return VariableDefine(Parser->pc, Parser, Ident, 0, Typ, 1);
    }
}


int VariableDefined(Picoc *pc, const char *Ident)
{
    struct Value *FoundValue;

    if (pc->TopStackFrame == 0 || !TableGet(&pc->TopStackFrame->LocalTable, Ident, &FoundValue, 0, 0, 0))
    {
        if (!TableGet(&pc->GlobalTable, Ident, &FoundValue, 0, 0, 0))
            return 0;
    }

    return 1;
}


void VariableGet(Picoc *pc, struct ParseState *Parser, const char *Ident, struct Value **LVal)
{
    if (pc->TopStackFrame == 0 || !TableGet(&pc->TopStackFrame->LocalTable, Ident, LVal, 0, 0, 0))
    {
        if (!TableGet(&pc->GlobalTable, Ident, LVal, 0, 0, 0))
        {
            if (VariableDefinedAndOutOfScope(pc, Ident))
                ProgramFail(Parser, "'%s' is out of scope", Ident);
            else
                ProgramFail(Parser, "'%s' is undefined", Ident);
        }
    }
}


void VariableDefinePlatformVar(Picoc *pc, struct ParseState *Parser, char *Ident, struct ValueType *Typ, union AnyValue *FromValue, int IsWritable)
{
    struct Value *SomeValue = VariableAllocValueAndData(pc, 0, 0, IsWritable, 0, 1);
    SomeValue->Typ = Typ;
    SomeValue->Val = FromValue;

    if (!TableSet(pc, (pc->TopStackFrame == 0) ? &pc->GlobalTable : &pc->TopStackFrame->LocalTable, TableStrRegister(pc, Ident), SomeValue, Parser ? Parser->FileName : 0, Parser ? Parser->Line : 0, Parser ? Parser->CharacterPos : 0))
        ProgramFail(Parser, "'%s' is already defined", Ident);
}


void VariableStackPop(struct ParseState *Parser, struct Value *Var)
{
    int Success;






    if (Var->ValOnHeap)
    {
        if (Var->Val != 0)
            HeapFreeMem(Parser->pc, Var->Val);

        Success = HeapPopStack(Parser->pc, Var, sizeof(struct Value));
    }
    else if (Var->ValOnStack)
        Success = HeapPopStack(Parser->pc, Var, sizeof(struct Value) + TypeSizeValue(Var, 0));
    else
        Success = HeapPopStack(Parser->pc, Var, sizeof(struct Value));

    if (!Success)
        ProgramFail(Parser, "stack underrun");
}


void VariableStackFrameAdd(struct ParseState *Parser, const char *FuncName, int NumParams)
{
    struct StackFrame *NewFrame;

    HeapPushStackFrame(Parser->pc);
    NewFrame = HeapAllocStack(Parser->pc, sizeof(struct StackFrame) + sizeof(struct Value *) * NumParams);
    if (NewFrame == 0)
        ProgramFail(Parser, "out of memory");

    ParserCopy(&NewFrame->ReturnParser, Parser);
    NewFrame->FuncName = FuncName;
    NewFrame->Parameter = (NumParams > 0) ? ((void *)((char *)NewFrame + sizeof(struct StackFrame))) : 0;
    TableInitTable(&NewFrame->LocalTable, &NewFrame->LocalHashTable[0], 11, 0);
    NewFrame->PreviousStackFrame = Parser->pc->TopStackFrame;
    Parser->pc->TopStackFrame = NewFrame;
}


void VariableStackFramePop(struct ParseState *Parser)
{
    if (Parser->pc->TopStackFrame == 0)
        ProgramFail(Parser, "stack is empty - can't go back");

    ParserCopy(Parser, &Parser->pc->TopStackFrame->ReturnParser);
    Parser->pc->TopStackFrame = Parser->pc->TopStackFrame->PreviousStackFrame;
    HeapPopStackFrame(Parser->pc);
}


struct Value *VariableStringLiteralGet(Picoc *pc, char *Ident)
{
    struct Value *LVal = 0;

    if (TableGet(&pc->StringLiteralTable, Ident, &LVal, 0, 0, 0))
        return LVal;
    else
        return 0;
}


void VariableStringLiteralDefine(Picoc *pc, char *Ident, struct Value *Val)
{
    TableSet(pc, &pc->StringLiteralTable, Ident, Val, 0, 0, 0);
}


void *VariableDereferencePointer(struct ParseState *Parser, struct Value *PointerValue, struct Value **DerefVal, int *DerefOffset, struct ValueType **DerefType, int *DerefIsLValue)
{
    if (DerefVal != 0)
        *DerefVal = 0;

    if (DerefType != 0)
        *DerefType = PointerValue->Typ->FromType;

    if (DerefOffset != 0)
        *DerefOffset = 0;

    if (DerefIsLValue != 0)
        *DerefIsLValue = 1;

    return PointerValue->Val->Pointer;
}
//# 12 "c-demos/gitlab.com-zsaleeba-picoc/main.c" 2
