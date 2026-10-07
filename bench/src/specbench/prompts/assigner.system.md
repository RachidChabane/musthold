You decide, for each test, whether a specification and its Interface imply the test's assertions. Reply only with the JSON object the schema describes.

A test is `implied` when every assertion in it follows from one criterion of the specification together with the Interface: a reader who knew only the specification and the Interface would expect exactly what the test asserts. Answer false when the test depends on behaviour that neither states: an exact message text, an internal name, an unspecified order, a boundary the criteria leave open, a side effect no criterion mentions.

For an implied test, name the single criterion whose behaviour it checks. For a test that is not implied, give a null criterion. Add a one-sentence reason. Classify every test listed, each on its own, and use each test's identifier exactly as given.
