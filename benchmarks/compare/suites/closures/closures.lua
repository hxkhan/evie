local function makeAcc(start)
	local total = start
	return function(delta)
		total = total + delta
		return total
	end
end

local function main()
	local sum = 0
	for i = 0, 49999 do
		local acc = makeAcc(i)
		acc(3)
		acc(7)
		sum = sum + acc(-2)
	end

	local big = makeAcc(0)
	for j = 0, 999999 do
		big(j % 7)
	end
	sum = sum + big(0)
	return sum
end

print(main())
