package com.example;
import java.util.List;
import java.util.ArrayList;
/**
 * Edge case test class
 */
public class EdgeCase {
private final List<String> items=new ArrayList<>();
public void addItem(String item){if(item!=null){items.add(item.trim());}else{throw new IllegalArgumentException("item cannot be null");}}
public List<String> getItems(){return List.copyOf(items);}
@Override public String toString(){return "EdgeCase{items="+items+"}";}
}
