def greet(name)
    "Hello, #{name}!"
end

class Animal
  attr_reader :name, :species

  def initialize(name, species)
    @name = name
    @species = species
  end

  def speak
    "#{@name} makes a sound."
  end

  def to_s
    "#{@name} (#{@species})"
  end
end

class Dog < Animal
  def initialize(name, breed)
    super(name, "Dog")
    @breed = breed
  end

  def speak
    "#{@name} barks: Woof!"
  end

  def fetch
    "#{@name} is fetching the ball."
  end
end

def factorial(n)
  return 1 if n <= 1
  n * factorial(n - 1)
end

dog = Dog.new("Buddy", "Golden Retriever")
puts greet(dog.name)
puts dog.speak
puts dog.fetch
puts "Factorial of 5: #{factorial(5)}"
