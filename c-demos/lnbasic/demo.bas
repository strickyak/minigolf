10 REM Line Number BASIC Demo Program
20 REM Computes triangular numbers and checks conditionals
30 PRINT "======================================"
40 PRINT "   LINE NUMBER BASIC IN ACTION"
50 PRINT "======================================"
60 LET S = 0
70 FOR I = 1 TO 10 STEP 2
80   S = S + I
90   PRINT "STEP I="; I; ", SUM="; S
100 NEXT I
110 IF S = 25 THEN GOTO 140
120 PRINT "TEST FAILED!"
130 GOTO 160
140 PRINT "TRIANGULAR ODD SUM VERIFIED: "; S
150 PRINT "SUCCESS!"
160 REM End of demo
