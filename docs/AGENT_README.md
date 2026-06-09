From now on, remember that the documents in `./docs` are authoritative, but may require updating at my direction.  
Current features are not fully defined and will require refinement.  


## DOCUMENT USAGE
Use the Senior Application Architect and Senior Document Witer experts to maintain documents.  Encourage experts to collaborate following this process:
- As we refine the features (example: applets) first identify all the related documentation.  
  - Review the related documents and evaluate the refinement in the context of the existing documentation 
  - If the new changes are ambiguous with the documents, report the ambiguity to the user and prompt for a resolution.  Accept the resolution and update all of the related documents appropriately.
  - Update the documents with terse but accurate language.  Use tables, schedules, links, and indeces to appropriately manage context but also maintain accuracy and architecture
### Apply Changes to Code After Documents Are Updated
Follow this process when applying changes:
- Apply changes
  - Changes are usually related to some change in behavior.  Capture this as a new or updated GHERKIN use-cases, and build hermetic unit tests around these.  Once hermetic unit tests are complete, tests may require Integration/Functional or End-To-End (E2E) tests.  Advise user when these are appropriate.  These must also be expressed and documented as GHERKIN use-cases.
    - Each hermetic unit test should include the GHERKIN use-case in its documentation.
    - Create tests to assert that the proposed changes are functional.
    - Strive for 98% or higher code coverage with hermetic unit tests.
- Apply quality gates
  - No Syntax Errors
  - Apply Best Practices for Linting appropriate to the language
  - All New Tests Must Pass 
- Update the CHANGELOG.md with the modified changes, and update the semantic version appropriately.
