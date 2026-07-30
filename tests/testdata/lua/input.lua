local function greet(name)
if name then
print("Hello, " .. name .. "!")
else
print("Hello, world!")
end
end
local fruits = {"apple", "banana", "cherry"}
for i, fruit in ipairs(fruits) do
print(i, fruit)
end
greet("Lua")
