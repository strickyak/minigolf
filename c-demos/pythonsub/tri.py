def tri(zero, unit, n):
    sum = zero
    count = zero
    for i in range(n):
        count = count + unit
        sum = sum + count
    return sum

print (tri(0, 1, 10))
print (tri([], [[]], 10))
