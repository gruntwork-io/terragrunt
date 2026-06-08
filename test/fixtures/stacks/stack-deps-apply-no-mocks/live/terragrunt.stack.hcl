# The consumer's autoinclude dependency defines NO mock_outputs. run --all apply must apply the
# producer first (DAG order) so the consumer reads the producer's real output without needing a mock.
unit "producer" {
  source = "../catalog/units/producer"
  path   = "producer"
}

unit "consumer" {
  source = "../catalog/units/consumer"
  path   = "consumer"

  autoinclude {
    dependency "producer" {
      config_path = unit.producer.path
    }

    inputs = {
      producer_val = dependency.producer.outputs.val
    }
  }
}
