# Rewrite the attached file provided

[CRITICAL] Retain the original code logic as-is, only augment the code by adding one **import** and the **class decorator** as described. IT IS CRITICALLY IMPORTANT TO RETAIN THE ORIGINAL APPLICATION CODE.  Treat this as an AST-preserving transformation. Do not reorder imports except to insert the new import. Do not reformat or alter comments/whitespace beyond what’s needed for the decorator insertion.

- **IMPORT**: add import for the class decorator: `from dagrr_core.common.etl_metadata.transformations import etl_step`. Insert the new import after existing imports and before constants/definitions. If the same import exists, do not add a duplicate.
- **CLASS DECORATOR**: If the 'etl_step' decorator is already present, do not add another; only update parameters if missing. If not present, add the `etl_step` class decorator to the class with a `modify()` function.  
  - *Requires*: Examine the `requires` property (it may be a property method) - this provides the `requires` List[str] parameter to the class decorator
  - *Description*: Provide a meaninful but brief description of the `modify` transformation (no more than three sentences) - this will be the `description` parameter value
  - *Operation*: Classify the nature of the `modify` function as a single word that best describes the SQL-nature of the transformation - this is the `operation` string parameter to the class decorator. [CRITICAL] Choose the word from the this list that best describes the nature of logic implemented within `modify`:
    - UPDATE: When a field is modified or added to a DataFrame using values derived by the method using data from the DataFrame (only one DataFrame involved)
    - DROP: When a field is dropped from a DataFrame (only one dataframe involved)
    - UNION: When multiple DataFrames are unioned to create a new DataFrame (even if there is logic that then removes some subset of records or fields)
    - INNER-JOIN: When multiple DataFrames are joined and their inner-join product is used to create a new DataFrame
    - LEFT-OUTER-JOIN: When multiple DataFrames are joined such that all records from the first DataFrame are preserved but records are augmented with fields from other DataFrames that join.
    - SUBSELECT: When a subset of a single DataFrame is returned as a new DataFrame
    - EXPORT: The DataFrame is exported, but not changed.
  - *Inputs*: Identify `modify` DataFrame input parameter names and map them to fields/columns used in the method - these provide the `inputs` List[Dict[List[str]]] parameter for the class decorator.  If all columns are used, use the value "*" to signify all columns
  - *Outputs*: Identify the DataFrames returned from `moodify` and map them to modified fields/columns - these provide the `outputs` List[Dict[str,List[str]]] parameter for the class decorator.  If all columns are used, use the value "*" to signify all columns
  - Deterime which fields/columns in the dataframe have been added, modified, and removed, and note their data type.
  - *Added, Modified, Removed*: Map each field with a brief (no more than 2 sentences) description of the field modification or properties, and add these as appropriate as `added`, `modified`, and `removed` dictionary parameters to the `etl_step` class decorator
  - *Depends_on*: Identify any fields used in logic conditional for the `modify` transformation logic and create a brief description of each - these will make the `depends_on` dictionary parameter

[CRITICAL] DO NOT REMOVE OR CHANGE EXISTING CLASSES, METHODS, IMPORTS OR COMMENTS.

OUTPUT POLICY: The only attachment must be the updated source file with the original relative path as name

[EXAMPLES - DO NOT REPRODUCE]

The following examples are **illustrative only**. Do NOT copy their count, headings, or structure in your final output.

**Example 1**: This example shows the inserted augmentation - this is the extent of change made by Agent:

    from dagrr_core.common.etl_metada.transformations import etl_step

    # Class Decorator added by the Agent because class has `modify` method
    @etl_step(
      description="Adds customer_age column based on birth_date",
      requires=["c.FLAT_CARD_UNREGISTER"]
      operation="UPDATE"
      inputs=[{"df_customers": ["customer_id", "birth_date"]}],
      outputs=[{"df_customers": ["customer_id", "birth_date", "customer_age"]}],
      added={"customer_age:int": "The duration since the customer's birthday"},
      modified={},
      removed={},
      depends_on={"birth_date": "the date the customer provided when they created their account"},
    )

**Example 2**: This shows the augmentation relative to **existing code** which is not modified by showing BEFORE and AFTER

INPUT:

  import numpy
  from typing import List

  class AddCustomerAgeColumn:
      @property
      def requires(self) -> List[str]:
          return [c.FLAT_CARD_UNREGISTER]

      def modify(self, df_customers):
          return df_customers

OUTPUT:

  import numpy
  from typing import List

  from dagrr_core.common.etl_metada.transformations import etl_step

  @etl_step(
        description="Adds customer_age column based on birth_date",
        requires=["c.FLAT_CARD_UNREGISTER"]
        operation="UPDATE"
        inputs=[{"df_customers": ["customer_id", "birth_date"]}],
        outputs=[{"df_customers": ["customer_id", "birth_date", "customer_age"]}],
        added={"customer_age:int": "The duration since the customer's birthday"},
        modified={},
        removed={},
        depends_on={"birth_date": "the date the customer provided when they created their account"},
    )
  class AddCustomerAgeColumn:
      @property
      def requires(self) -> List[str]:
          return [c.FLAT_CARD_UNREGISTER]

      def modify(self, df_customers):
          return df_customers

[END EXAMPLES]
